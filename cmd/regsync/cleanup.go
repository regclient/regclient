package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"

	"github.com/regclient/regclient/types/ref"
)

// matchesExclusionPattern checks if a tag matches any exclusion pattern.
// Returns true if the tag matches, along with the matching pattern for logging.
// Returns an error if any regex pattern fails to compile.
func matchesExclusionPattern(tag string, patterns []string) (bool, string, error) {
	if len(patterns) == 0 {
		return false, "", nil
	}

	// Check each pattern
	for _, pattern := range patterns {
		exp, err := regexp.Compile(pattern)
		if err != nil {
			return false, "", fmt.Errorf("invalid exclusion pattern %q: %w", pattern, err)
		}
		if exp.MatchString(tag) {
			return true, pattern, nil
		}
	}

	return false, "", nil
}

// cleanupTags removes tags from target repository that don't match filters
func (opts *rootOpts) cleanupTags(ctx context.Context, s ConfigSync, tgt string) error {
	// Parse target reference
	tgtRef, err := ref.New(tgt)
	if err != nil {
		opts.log.Error("Failed parsing target for cleanup",
			slog.String("target", tgt),
			slog.String("error", err.Error()))
		return err
	}

	// Retrieve all tags from target repository
	tTags, err := opts.rc.TagList(ctx, tgtRef)
	if err != nil {
		opts.log.Error("Failed getting target tags for cleanup",
			slog.String("target", tgtRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}
	tTagsList, err := tTags.GetTags()
	if err != nil {
		opts.log.Error("Failed getting target tags for cleanup",
			slog.String("target", tgtRef.CommonName()),
			slog.String("error", err.Error()))
		return err
	}

	// Build list of "wanted" tags using the same filter logic as sync
	sets := s.TagSets
	if len(s.Tags.Allow) > 0 || len(s.Tags.Deny) > 0 || len(s.Tags.SemverRange) > 0 {
		sets = append(sets, s.Tags)
	}
	wantedTags := []string{}
	if len(sets) == 0 {
		// No filters means all tags are wanted
		wantedTags = tTagsList
	} else {
		for _, set := range sets {
			filteredCur, err := filterTagList(set, tTagsList)
			if err != nil {
				opts.log.Error("Failed processing tag filters for cleanup",
					slog.String("target", tgtRef.CommonName()),
					slog.Any("allow", set.Allow),
					slog.Any("deny", set.Deny),
					slog.Any("semverRange", set.SemverRange),
					slog.String("error", err.Error()))
				return err
			}
			// Add unique tags to wanted list
			for _, tag := range filteredCur {
				if !slices.Contains(wantedTags, tag) {
					wantedTags = append(wantedTags, tag)
				}
			}
		}
	}

	// Identify tags to delete
	tagsToDelete := []string{}
	for _, tag := range tTagsList {
		// Check if tag is wanted (matches filters)
		if slices.Contains(wantedTags, tag) {
			continue
		}

		// Check if tag matches exclusion patterns
		excluded, pattern, err := matchesExclusionPattern(tag, s.CleanupTagsExclude)
		if err != nil {
			opts.log.Error("Failed checking exclusion pattern",
				slog.String("target", tgtRef.CommonName()),
				slog.String("tag", tag),
				slog.String("error", err.Error()))
			return err
		}
		if excluded {
			opts.log.Debug("Tag excluded from cleanup",
				slog.String("target", tgtRef.CommonName()),
				slog.String("tag", tag),
				slog.String("pattern", pattern))
			continue
		}

		// Tag should be deleted
		tagsToDelete = append(tagsToDelete, tag)
	}

	// Delete unwanted tags
	errs := []error{}
	for _, tag := range tagsToDelete {
		// Check context before each deletion
		select {
		case <-ctx.Done():
			errs = append(errs, ErrCanceled)
			return errors.Join(errs...)
		default:
		}

		opts.log.Info("Deleting tag",
			slog.String("target", tgtRef.CommonName()),
			slog.String("tag", tag))

		tagRef := tgtRef.SetTag(tag)
		err := opts.rc.TagDelete(ctx, tagRef)
		if err != nil {
			opts.log.Error("Failed to delete tag",
				slog.String("target", tgtRef.CommonName()),
				slog.String("tag", tag),
				slog.String("error", err.Error()))
			errs = append(errs, fmt.Errorf("failed to delete tag %s:%s: %w", tgtRef.CommonName(), tag, err))
		} else {
			opts.log.Debug("Deleted tag",
				slog.String("target", tgtRef.CommonName()),
				slog.String("tag", tag))
		}
	}

	if len(tagsToDelete) == 0 {
		opts.log.Debug("No tags require cleanup",
			slog.String("target", tgtRef.CommonName()))
	}

	return errors.Join(errs...)
}
