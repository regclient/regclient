package regsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient"
	"github.com/regclient/regclient/internal/pqueue"
	"github.com/regclient/regclient/internal/semver"
	"github.com/regclient/regclient/pkg/template"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

// throttle is used for limiting concurrent sync steps from running.
// This is separate from the concurrency limits in regclient itself.
type Throttle struct{}

type Regsync struct {
	log        *slog.Logger
	rc         *regclient.RegClient
	throttle   *pqueue.Queue[Throttle]
	abortOnErr bool
}

// Opt functions are used by [New] to create a [*RegClient].
type Opt func(*Regsync)

func WithAbortOnErr(abortOnErr bool) Opt {
	return func(rs *Regsync) {
		rs.abortOnErr = abortOnErr
	}
}

func WithThrottle(throttle *pqueue.Queue[Throttle]) Opt {
	return func(rs *Regsync) {
		rs.throttle = throttle
	}
}

func New(rc *regclient.RegClient, opts ...Opt) *Regsync {
	rs := Regsync{
		rc:  rc,
		log: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})),
	}
	for _, opt := range opts {
		opt(&rs)
	}
	return &rs
}

// process a sync step
func (rs *Regsync) Process(ctx context.Context, s ConfigSync, action ActionType) error {
	switch s.Type {
	case "registry":
		if err := rs.processRegistry(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	case "repository":
		if err := rs.processRepo(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	case "image":
		if err := rs.processImage(ctx, s, s.Source, s.Target, action); err != nil {
			return err
		}
	default:
		rs.log.Error("Type not recognized, must be one of: registry, repository, or image",
			slog.Any("step", s),
			slog.String("type", s.Type))
		return ErrInvalidInput
	}
	return nil
}

func (rs *Regsync) processRegistry(ctx context.Context, s ConfigSync, src, tgt string, action ActionType) error {
	last := ""
	errs := []error{}
	// loop through pages of the _catalog response
	for {
		repoOpts := []scheme.RepoOpts{}
		if last != "" {
			repoOpts = append(repoOpts, scheme.WithRepoLast(last))
		}
		sRepos, err := rs.rc.RepoList(ctx, src, repoOpts...)
		if err != nil {
			rs.log.Error("Failed to list source repositories",
				slog.String("source", src),
				slog.String("error", err.Error()))
			return err
		}
		sRepoList, err := sRepos.GetRepos()
		if err != nil {
			rs.log.Error("Failed to list source repositories",
				slog.String("source", src),
				slog.String("error", err.Error()))
			return err
		}
		if len(sRepoList) == 0 || last == sRepoList[len(sRepoList)-1] {
			break
		}
		last = sRepoList[len(sRepoList)-1]
		// filter repos according to allow/deny rules
		sRepoList, err = filterRepoList(s.Repos, sRepoList)
		if err != nil {
			rs.log.Error("Failed processing repo filters",
				slog.String("source", src),
				slog.Any("allow", s.Repos.Allow),
				slog.Any("deny", s.Repos.Deny),
				slog.String("error", err.Error()))
			return err
		}
		for _, repo := range sRepoList {
			if err := rs.processRepo(ctx, s, fmt.Sprintf("%s/%s", src, repo), fmt.Sprintf("%s/%s", tgt, repo), action); err != nil {
				errs = append(errs, err)
				if rs.abortOnErr {
					break
				}
			}
		}
		if rs.abortOnErr && len(errs) > 0 {
			break
		}
	}
	return errors.Join(errs...)
}

func (rs *Regsync) processRepo(ctx context.Context, s ConfigSync, src, tgt string, action ActionType) error {
	sRepoRef, err := ref.New(src)
	if err != nil {
		rs.log.Error("Failed parsing source",
			slog.String("source", src),
			slog.String("error", err.Error()))
		return err
	}
	sTags, err := rs.rc.TagList(ctx, sRepoRef)
	if err != nil {
		rs.log.Error("Failed getting source tags",
			slog.String("source", sRepoRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	sTagsList, err := sTags.GetTags()
	if err != nil {
		rs.log.Error("Failed getting source tags",
			slog.String("source", sRepoRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	sets := s.TagSets
	if len(s.Tags.Allow) > 0 || len(s.Tags.Deny) > 0 || len(s.Tags.SemverRange) > 0 {
		sets = append(sets, s.Tags)
	}
	sTagsFiltered := []string{}
	if len(sets) == 0 {
		// no filters includes all tags
		sTagsFiltered = sTagsList
	}
	for _, set := range sets {
		sFilteredCur, err := FilterTagList(set, sTagsList)
		if err != nil {
			rs.log.Error("Failed processing tag filters",
				slog.String("source", sRepoRef.CommonName()),
				slog.Any("allow", set.Allow),
				slog.Any("deny", set.Deny),
				slog.Any("semverRange", set.SemverRange),
				slog.String("error", err.Error()))
			return err
		}
		if len(sTagsFiltered) == 0 {
			sTagsFiltered = sFilteredCur
		} else {
			// add unique tags
			for _, tag := range sFilteredCur {
				if !slices.Contains(sTagsFiltered, tag) {
					sTagsFiltered = append(sTagsFiltered, tag)
				}
			}
		}
	}
	if len(sTagsFiltered) == 0 {
		rs.log.Warn("No matching tags found",
			slog.String("source", sRepoRef.CommonName()),
			slog.Any("tags", s.Tags),
			slog.Any("tagSets", s.TagSets),
			slog.Any("available", sTagsList))
		return nil
	}
	// if only copying missing entries, delete tags that already exist on target
	if action == ActionMissing {
		tRepoRef, err := ref.New(tgt)
		if err != nil {
			rs.log.Error("Failed parsing target",
				slog.String("target", tgt),
				slog.String("error", err.Error()))
			return err
		}
		tTags, err := rs.rc.TagList(ctx, tRepoRef)
		if err != nil {
			rs.log.Debug("Failed getting target tags",
				slog.String("target", tRepoRef.CommonName()),
				slog.String("error", err.Error()))
		}
		tTagList := []string{}
		if err == nil {
			tTagList, err = tTags.GetTags()
			if err != nil {
				rs.log.Debug("Failed getting target tags",
					slog.String("target", tRepoRef.CommonName()),
					slog.String("error", err.Error()))
			}
		}
		slices.Sort(sTagsFiltered)
		slices.Sort(tTagList)
		sI := len(sTagsFiltered) - 1
		tI := len(tTagList) - 1
		for sI >= 0 && tI >= 0 {
			switch strings.Compare(sTagsFiltered[sI], tTagList[tI]) {
			case 0:
				sTagsFiltered = slices.Delete(sTagsFiltered, sI, sI+1)
				sI--
				tI--
			case -1:
				tI--
			case 1:
				sI--
			default:
				rs.log.Warn("strings.Compare unexpected result",
					slog.Int("result", strings.Compare(sTagsFiltered[sI], tTagList[tI])),
					slog.String("left", sTagsFiltered[sI]),
					slog.String("right", tTagList[tI]))
				sI--
				tI--
			}
		}
	}
	errs := []error{}
	for _, tag := range sTagsFiltered {
		if err := rs.processImage(ctx, s, fmt.Sprintf("%s:%s", src, tag), fmt.Sprintf("%s:%s", tgt, tag), action); err != nil {
			errs = append(errs, err)
			if rs.abortOnErr {
				break
			}
		}
	}
	return errors.Join(errs...)
}

func (rs *Regsync) processImage(ctx context.Context, s ConfigSync, src, tgt string, action ActionType) error {
	sRef, err := ref.New(src)
	if err != nil {
		rs.log.Error("Failed parsing source",
			slog.String("source", src),
			slog.String("error", err.Error()))
		return err
	}
	tRef, err := ref.New(tgt)
	if err != nil {
		rs.log.Error("Failed parsing target",
			slog.String("target", tgt),
			slog.String("error", err.Error()))
		return err
	}
	err = rs.ProcessRef(ctx, s, sRef, tRef, action)
	if err != nil {
		rs.log.Error("Failed to sync",
			slog.String("target", tRef.CommonName()),
			slog.String("source", sRef.CommonName()),
			slog.String("error", err.Error()))
	}
	if err := rs.rc.Close(ctx, tRef); err != nil {
		rs.log.Error("Error closing ref",
			slog.String("ref", tRef.CommonName()),
			slog.String("error", err.Error()))
	}
	return err
}

// process a sync step
func (rs *Regsync) ProcessRef(ctx context.Context, s ConfigSync, src, tgt ref.Ref, action ActionType) error {
	mSrc, err := rs.rc.ManifestHead(ctx, src, regclient.WithManifestRequireDigest())
	if err != nil && errors.Is(err, errs.ErrUnsupportedAPI) {
		mSrc, err = rs.rc.ManifestGet(ctx, src)
	}
	if err != nil {
		rs.log.Error("Failed to lookup source manifest",
			slog.String("source", src.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	fastCheck := (s.FastCheck != nil && *s.FastCheck)
	forceRecursive := (s.ForceRecursive != nil && *s.ForceRecursive)
	referrers := (s.Referrers != nil && *s.Referrers)
	digestTags := (s.DigestTags != nil && *s.DigestTags)
	mTgt, err := rs.rc.ManifestHead(ctx, tgt, regclient.WithManifestRequireDigest())
	tgtExists := (err == nil)
	tgtMatches := false
	if err == nil && manifest.GetDigest(mSrc).String() == manifest.GetDigest(mTgt).String() {
		tgtMatches = true
	}
	if tgtMatches && (fastCheck || (!forceRecursive && !referrers && !digestTags)) {
		rs.log.Debug("Image matches",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
		return nil
	}
	if tgtExists && action == ActionMissing {
		rs.log.Debug("target exists",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
		return nil
	}

	// skip when source manifest is an unsupported type
	smt := manifest.GetMediaType(mSrc)
	if !slices.Contains(s.MediaTypes, smt) {
		rs.log.Info("Skipping unsupported media type",
			slog.String("ref", src.CommonName()),
			slog.String("mediaType", manifest.GetMediaType(mSrc)),
			slog.Any("allowed", s.MediaTypes))
		return nil
	}

	// if platform is defined and source is a list, resolve the source platform
	if mSrc.IsList() && s.Platform != "" {
		platDigest, err := rs.getPlatformDigest(ctx, src, s.Platform, mSrc)
		if err != nil {
			return err
		}
		src.Digest = platDigest.String()
		if tgtExists && platDigest.String() == manifest.GetDigest(mTgt).String() {
			tgtMatches = true
		}
		if tgtMatches && (s.ForceRecursive == nil || !*s.ForceRecursive) {
			rs.log.Debug("Image matches for platform",
				slog.String("source", src.CommonName()),
				slog.String("platform", s.Platform),
				slog.String("target", tgt.CommonName()))
			return nil
		}
	}
	if tgtMatches {
		rs.log.Info("Image refreshing",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()),
			slog.Bool("forced", forceRecursive),
			slog.Bool("digestTags", digestTags),
			slog.Bool("referrers", referrers))
	} else {
		rs.log.Info("Image sync needed",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()))
	}
	if action == ActionCheck {
		return nil
	}

	// wait for parallel tasks
	throttleDone, err := rs.throttle.Acquire(ctx, Throttle{})
	if err != nil {
		return fmt.Errorf("failed to acquire throttle: %w", err)
	}
	// delay for rate limit on source
	if s.RateLimit.Min > 0 && manifest.GetRateLimit(mSrc).Set {
		// refresh current rate limit after acquiring throttle
		mSrc, err = rs.rc.ManifestHead(ctx, src)
		if err != nil {
			rs.log.Error("rate limit check failed",
				slog.String("source", src.CommonName()),
				slog.String("error", err.Error()))
			throttleDone()
			return err
		}
		// delay if rate limit exceeded
		rlSrc := manifest.GetRateLimit(mSrc)
		for rlSrc.Remain < s.RateLimit.Min {
			throttleDone()
			rs.log.Info("Delaying for rate limit",
				slog.String("source", src.CommonName()),
				slog.Int("source-remain", rlSrc.Remain),
				slog.Int("source-limit", rlSrc.Limit),
				slog.Int("step-min", s.RateLimit.Min),
				slog.Duration("sleep", s.RateLimit.Retry))
			select {
			case <-ctx.Done():
				return ErrCanceled
			case <-time.After(s.RateLimit.Retry):
			}
			throttleDone, err = rs.throttle.Acquire(ctx, Throttle{})
			if err != nil {
				return fmt.Errorf("failed to reacquire throttle: %w", err)
			}
			mSrc, err = rs.rc.ManifestHead(ctx, src)
			if err != nil {
				rs.log.Error("rate limit check failed",
					slog.String("source", src.CommonName()),
					slog.String("error", err.Error()))
				throttleDone()
				return err
			}
			rlSrc = manifest.GetRateLimit(mSrc)
		}
		rs.log.Debug("Rate limit passed",
			slog.String("source", src.CommonName()),
			slog.Int("source-remain", rlSrc.Remain),
			slog.Int("step-min", s.RateLimit.Min))
	}
	defer throttleDone()

	// verify context has not been canceled while waiting for throttle
	select {
	case <-ctx.Done():
		return ErrCanceled
	default:
	}

	// run backup
	if tgtExists && !tgtMatches && s.Backup != "" {
		// expand template
		data := struct {
			Ref  ref.Ref
			Step ConfigSync
			Sync ConfigSync
		}{Ref: tgt, Step: s, Sync: s}
		backupStr, err := template.String(s.Backup, data)
		if err != nil {
			rs.log.Error("Failed to expand backup template",
				slog.String("original", tgt.CommonName()),
				slog.String("backup-template", s.Backup),
				slog.String("error", err.Error()))
			return err
		}
		backupStr = strings.TrimSpace(backupStr)
		backupRef := tgt
		if strings.ContainsAny(backupStr, ":/") {
			// if the : or / are in the string, parse it as a full reference
			backupRef, err = ref.New(backupStr)
			if err != nil {
				rs.log.Error("Failed to parse backup reference",
					slog.String("original", tgt.CommonName()),
					slog.String("template", s.Backup),
					slog.String("backup", backupStr),
					slog.String("error", err.Error()))
				return err
			}
		} else {
			// else parse backup string as just a tag
			backupRef = backupRef.SetTag(backupStr)
		}
		defer rs.rc.Close(ctx, backupRef)
		// run copy from tgt ref to backup ref
		rs.log.Info("Saving backup",
			slog.String("original", tgt.CommonName()),
			slog.String("backup", backupRef.CommonName()))
		err = rs.rc.ImageCopy(ctx, tgt, backupRef)
		if err != nil {
			// Possible registry corruption with existing image, only warn and continue/overwrite
			rs.log.Warn("Failed to backup existing image",
				slog.String("original", tgt.CommonName()),
				slog.String("template", s.Backup),
				slog.String("backup", backupRef.CommonName()),
				slog.String("error", err.Error()))
		}
	}

	rcOpts := []regclient.ImageOpts{}
	if s.DigestTags != nil && *s.DigestTags {
		rcOpts = append(rcOpts, regclient.ImageWithDigestTags())
	}
	if s.Referrers != nil && *s.Referrers {
		if len(s.ReferrerFilters) == 0 {
			rcOpts = append(rcOpts, regclient.ImageWithReferrers())
		} else {
			for _, filter := range s.ReferrerFilters {
				rOpts := []scheme.ReferrerOpts{}
				if filter.ArtifactType != "" {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{ArtifactType: filter.ArtifactType}))
				}
				if filter.Annotations != nil {
					rOpts = append(rOpts, scheme.WithReferrerMatchOpt(descriptor.MatchOpt{Annotations: filter.Annotations}))
				}
				if s.ReferrerSlow != nil && *s.ReferrerSlow {
					rOpts = append(rOpts, scheme.WithReferrerSlowSearch())
				}
				rcOpts = append(rcOpts, regclient.ImageWithReferrers(rOpts...))
			}
		}
		if s.ReferrerSrc != "" {
			referrerSrc, err := ref.New(s.ReferrerSrc)
			if err != nil {
				rs.log.Error("failed to parse referrer source reference",
					slog.String("referrerSource", s.ReferrerSrc),
					slog.String("error", err.Error()))
			}
			rcOpts = append(rcOpts, regclient.ImageWithReferrerSrc(referrerSrc))
		}
		if s.ReferrerTgt != "" {
			referrerTgt, err := ref.New(s.ReferrerTgt)
			if err != nil {
				rs.log.Error("failed to parse referrer target reference",
					slog.String("referrerTarget", s.ReferrerTgt),
					slog.String("error", err.Error()))
			}
			rcOpts = append(rcOpts, regclient.ImageWithReferrerTgt(referrerTgt))
		}
	}
	if s.FastCheck != nil && *s.FastCheck {
		rcOpts = append(rcOpts, regclient.ImageWithFastCheck())
	}
	if s.ForceRecursive != nil && *s.ForceRecursive {
		rcOpts = append(rcOpts, regclient.ImageWithForceRecursive())
	}
	if s.IncludeExternal != nil && *s.IncludeExternal {
		rcOpts = append(rcOpts, regclient.ImageWithIncludeExternal())
	}
	if len(s.Platforms) > 0 {
		rcOpts = append(rcOpts, regclient.ImageWithPlatforms(s.Platforms))
	}

	// Copy the image
	rs.log.Debug("Image sync running",
		slog.String("source", src.CommonName()),
		slog.String("target", tgt.CommonName()))
	err = rs.rc.ImageCopy(ctx, src, tgt, rcOpts...)
	if err != nil {
		rs.log.Error("Failed to copy image",
			slog.String("source", src.CommonName()),
			slog.String("target", tgt.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	return nil
}

// filterByRegex applies allow/deny regex patterns to a list of strings.
// filterRegexAllow returns items that match at least one allow pattern.
// If no patterns are provided, returns all items.
func filterRegexAllow(patterns []string, in []string) ([]string, error) {
	if len(patterns) == 0 {
		return in, nil
	}

	// Compile all patterns
	exps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		exp, err := regexp.Compile("^" + pattern + "$")
		if err != nil {
			return nil, fmt.Errorf("invalid allow pattern %q: %w", pattern, err)
		}
		exps = append(exps, exp)
	}

	// Keep items matching any pattern
	result := make([]string, 0, len(in))
	for _, item := range in {
		for _, exp := range exps {
			if exp.MatchString(item) {
				result = append(result, item)
				break
			}
		}
	}
	return result, nil
}

// filterRegexDeny removes items that match any deny pattern.
// If no patterns are provided, returns all items unchanged.
func filterRegexDeny(patterns []string, in []string) ([]string, error) {
	if len(patterns) == 0 {
		return in, nil
	}

	// Compile all patterns
	exps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		exp, err := regexp.Compile("^" + pattern + "$")
		if err != nil {
			return nil, fmt.Errorf("invalid deny pattern %q: %w", pattern, err)
		}
		exps = append(exps, exp)
	}

	// Remove items matching any pattern
	result := make([]string, 0, len(in))
	for _, item := range in {
		denied := false
		for _, exp := range exps {
			if exp.MatchString(item) {
				denied = true
				break
			}
		}
		if !denied {
			result = append(result, item)
		}
	}
	return result, nil
}

func filterRepoList(ad RepoAllowDeny, in []string) ([]string, error) {
	// Apply allow filter
	result, err := filterRegexAllow(ad.Allow, in)
	if err != nil {
		return nil, err
	}
	// Apply deny filter
	return filterRegexDeny(ad.Deny, result)
}

func FilterTagList(ad TagAllowDeny, in []string) ([]string, error) {
	result := in

	// Step 1: Apply semverRange filter
	if len(ad.SemverRange) > 0 {
		// Parse all constraints
		constraints := make([]semver.Constraint, 0, len(ad.SemverRange))
		for _, rangeStr := range ad.SemverRange {
			if rangeStr == "" {
				continue
			}
			constraint, err := semver.NewConstraint(rangeStr)
			if err != nil {
				return nil, fmt.Errorf("invalid semver range %q: %w", rangeStr, err)
			}
			constraints = append(constraints, constraint)
		}

		// Apply version filtering if we have valid constraints
		if len(constraints) > 0 {
			filtered := make([]string, 0, len(result))
			for _, tag := range result {
				// Try to parse as semver, skip non-semver tags
				v, err := semver.NewVersion(tag)
				if err != nil {
					continue
				}
				// Check if version matches any constraint (OR logic)
				for _, constraint := range constraints {
					if constraint.Check(v) {
						filtered = append(filtered, tag)
						break
					}
				}
			}
			result = filtered
		}
	}

	// Step 2: Apply Allow filter
	if len(ad.Allow) > 0 {
		var err error
		result, err = filterRegexAllow(ad.Allow, result)
		if err != nil {
			return nil, err
		}
	}

	// Step 3: Apply Deny filter
	return filterRegexDeny(ad.Deny, result)
}

var manifestCache struct {
	mu        sync.Mutex
	manifests map[string]manifest.Manifest
}

func init() {
	manifestCache.manifests = map[string]manifest.Manifest{}
}

// getPlatformDigest resolves a manifest list to a specific platform's digest
// This uses the above cache to only call ManifestGet when a new manifest list digest is seen
func (rs *Regsync) getPlatformDigest(ctx context.Context, r ref.Ref, platStr string, origMan manifest.Manifest) (digest.Digest, error) {
	plat, err := platform.Parse(platStr)
	if err != nil {
		rs.log.Warn("Could not parse platform",
			slog.String("platform", platStr),
			slog.String("err", err.Error()))
		return "", err
	}
	// cache manifestGet response
	manifestCache.mu.Lock()
	getMan, ok := manifestCache.manifests[manifest.GetDigest(origMan).String()]
	if !ok {
		getMan, err = rs.rc.ManifestGet(ctx, r)
		if err != nil {
			rs.log.Error("Failed to get source manifest",
				slog.String("source", r.CommonName()),
				slog.String("error", err.Error()))
			manifestCache.mu.Unlock()
			return "", err
		}
		manifestCache.manifests[manifest.GetDigest(origMan).String()] = getMan
	}
	manifestCache.mu.Unlock()
	descPlat, err := manifest.GetPlatformDesc(getMan, &plat)
	if err != nil {
		pl, _ := manifest.GetPlatformList(getMan)
		var ps []string
		for _, p := range pl {
			ps = append(ps, p.String())
		}
		rs.log.Warn("Platform could not be found in source manifest list",
			slog.String("platform", plat.String()),
			slog.String("err", err.Error()),
			slog.String("platforms", strings.Join(ps, ", ")))
		return "", ErrNotFound
	}
	return descPlat.Digest, nil
}
