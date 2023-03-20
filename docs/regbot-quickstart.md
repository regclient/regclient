# regbot Quick Start

## Setup a Registry

```shell
docker network create registry
docker run -d --restart=unless-stopped --name registry --net registry \
  -e "REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY=/var/lib/registry" \
  -e "REGISTRY_STORAGE_DELETE_ENABLED=true" \
  -e "REGISTRY_VALIDATION_DISABLED=true" \
  -v "registry-data:/var/lib/registry" \
  -p "127.0.0.1:5000:5000" \
  registry:2
```

## Configure the Yaml File

Create a file called `regbot.yml`:

```yaml
version: 1
creds:
  - registry: registry:5000
    tls: disabled
  - registry: docker.io
    user: "{{env \"HUB_USER\"}}"
    pass: "{{file \"/home/appuser/.docker/hub_token\"}}"
defaults:
  parallel: 2
  interval: 60m
  timeout: 600s
scripts:
  - name: mirror minor versions
    timeout: 59m
    script: |
      imageList = {"library/alpine", "library/debian"}
      localReg = "registry:5000"
      tagExp = "^%d+%.%d+$"
      minRateLimit = 100
      maxKeep = 3

      -- define functions for semver sorting
      function string.split (inputStr, sep)
        if sep == nil then
          sep = "%s"
        end
        local t={}
        for str in string.gmatch(inputStr, "([^"..sep.."]+)") do
          table.insert(t, str)
        end
        return t
      end
      function isNumber (inputStr)
        if string.gmatch(inputStr, "^%d+$") then
          return true
        else
          return false
        end
      end
      function orderSemVer(a, b)
        aSplit = string.split(a, "%.-")
        bSplit = string.split(b, "%.-")
        min = (#aSplit > #bSplit) and #aSplit or #bSplit
        for i = 1, min do
          if isNumber(aSplit[i]) and isNumber(bSplit[i]) then
            aNum = tonumber(aSplit[i])
            bNum = tonumber(bSplit[i])
            if aNum ~= bNum then
              return aNum < bNum
            end
          elseif aSplit[i] ~= bSplit[i] then
            return aSplit[i] < bSplit[i]
          end
        end
        -- TODO: should check for rc/alpha/beta versions on longer string (sorting before GA), and 0 is the same as no value
        return #aSplit < #bSplit
      end

      -- loop through images
      for k, imageName in ipairs(imageList) do
        upstreamRef = reference.new(imageName)
        localRef = reference.new(localReg .. "/" .. imageName)
        -- loop through tags on each image
        tags = tag.ls(upstreamRef)
        matchTags = {}
        for k, t in pairs(tags) do
          if string.match(t, tagExp) then
            table.insert(matchTags, t)
          end
        end
        table.sort(matchTags, orderSemVer)
        if #matchTags > maxKeep then
          matchTags = {unpack(matchTags, #matchTags - maxKeep + 1)}
        end
        for k, t in ipairs(matchTags) do
          -- only copy tags matching the expression
          if string.match(t, tagExp) then
            upstreamRef:tag(t)
            localRef:tag(t)
            if not image.ratelimitWait(upstreamRef, minRateLimit) then
              error "Timed out waiting on rate limit"
            end
            image.copy(upstreamRef, localRef)
          end
        end
      end

  - name: debian date stamped mirror
    timeout: 59m
    script: |
      imageName = "library/debian"
      tagExp = "^testing%-%d+$"
      maxKeep = 3
      minRateLimit = 150

      upstreamRef = reference.new(imageName)
      localRef = reference.new("registry:5000/" .. imageName)

      -- first copy new date stamped images
      tags = tag.ls(upstreamRef)
      matchTags = {}
      for k, t in pairs(tags) do
        if string.match(t, tagExp) then
          table.insert(matchTags, t)
        end
      end
      table.sort(matchTags)
      if #matchTags > maxKeep then
        matchTags = {unpack(matchTags, #matchTags - maxKeep + 1)}
        for k, t in ipairs(matchTags) do
          upstreamRef:tag(t)
          localRef:tag(t)
          if not image.ratelimitWait(upstreamRef, minRateLimit) then
            error "Timed out waiting on rate limit"
          end
          log("Copying " .. t)
          image.copy(upstreamRef, localRef)
        end
      end

      -- next delete old date stamped images
      tags = tag.ls(localRef)
      matchTags = {}
      for k, t in pairs(tags) do
        if string.match(t, tagExp) then
          table.insert(matchTags, t)
        end
      end
      table.sort(matchTags)
      if #matchTags > maxKeep then
        matchTags = {unpack(matchTags, 1, #matchTags - maxKeep)}
        for k, t in ipairs(matchTags) do
          localRef:tag(t)
          log("Deleting " .. t)
          tag.delete(localRef)
        end
      end

  - name: delete old builds
    script: |
      imageName = "registry:5000/regclient/example"
      tagExp = "^ci%-%d+$"
      maxDays = 30
      imageLabel = "org.opencontainers.image.created"
      dateFmt = "!%Y-%m-%dT%H:%M:%SZ"

      timeRef = os.time() - (86400*maxDays)
      cutoff = os.date(dateFmt, timeRef)
      log("Searching for images before: " .. cutoff)
      ref = reference.new(imageName)
      tags = tag.ls(ref)
      table.sort(tags)
      for k, t in pairs(tags) do
        if string.match(t, tagExp) then
          ref:tag(t)
          ic = image.config(ref)
          if ic.Config.Labels[imageLabel] < cutoff then
            log("Deleting " .. t .. " created on " .. ic.Config.Labels[imageLabel])
            tag.delete(ref)
          else
            log("Skipping " .. t .. " created on " .. ic.Config.Labels[imageLabel])
          end
        end
      end
```

This file contains three scripts to run every hour:

- mirror minor versions: This copies the 3 most recent minor versions from the
  upstream repositories for Alpine and Debian, copying them to
  `registry:5000/library/alpine` and `registry:5000/library/debian`
  respectively. This shows how you can copy images parsing the semver and
  including a specific range of those images.
- debian date stamped mirror: This copies the 3 most recent `testing-` tags
  followed by a number. A similar pattern could be used to copy nightly builds
  of an image. This also includes a cleanup of older images, avoiding filling
  the local registry with old images.
- delete old builds: This examines the example images (setup below), looking for
  anything matching the `ci-` tag followed by a number, and comparing the
  datestamp in the "org.opencontainers.image.created" label to see if it's more
  than 30 days old. This example could be used to automatically prune old builds
  that may not have been promoted through the CI pipeline and are no longer
  useful.

This file assumes the local registry is `registry:5000`, which is available when
using container-to-container networking. If you run this without container
networking, then adjust the local registry name and any configuration details to
access that registry with write access.

## Setup regctl

Run the following to setup `regctl` that will be used to setup and later inspect
the registry.

```shell
cat >regctl <<EOF
opts=""
case "\$*" in
  "registry login"*) opts="-t";;
esac
docker container run \$opts -i --rm --net host \\
  -u "\$(id -u):\$(id -g)" -e HOME -v \$HOME:\$HOME \\
  -v /etc/docker/certs.d:/etc/docker/certs.d:ro \\
  ghcr.io/regclient/regctl:latest "\$@"
EOF
chmod 755 regctl
./regctl registry set --tls disabled localhost:5000
```

## Load example images

Copy some images to the local registry that have the specified label to see the
"delete old builds" action work. The `ghcr.io/regclient/regctl` image includes these
labels, and we can also use `regctl` itself to copy these images.

```shell
./regctl image copy -v info \
    ghcr.io/regclient/regctl:v0.0.1 localhost:5000/regclient/example:latest \
&& \
./regctl image copy -v info \
    localhost:5000/regclient/example:latest localhost:5000/regclient/example:ci-001 \
&& \
./regctl image copy -v info \
    localhost:5000/regclient/example:latest localhost:5000/regclient/example:ci-002 \
&& \
./regctl image copy -v info \
    localhost:5000/regclient/example:latest localhost:5000/regclient/example:ci-003 \
&& \
./regctl image copy -v info \
    localhost:5000/regclient/example:latest localhost:5000/regclient/example:stable \
&& \
./regctl image copy -v info \
    library/debian:latest localhost:5000/library/debian:latest \
&& \
./regctl tag ls localhost:5000/regclient/example
```

The last command should show the tags for 3 CI images, latest, and stable. The
same old image was used for each, to minimize how many manifests were pulled
from Docker Hub. For a more realistic example you could copy unique images for
each. If you use other images, be sure the image label and date format match the
variables in the "delete old builds" script.

## Perform a Dry-Run

Run regbot in the "once" mode with the "dry-run" option to test the scripts.
Make sure to replace `your_username` with your Hub username and create a
`${HOME}/.docker/hub_token` file with your hub password or personal access
token.

```shell
export HUB_USER=your_hub_username
mkdir -p ${HOME}/.docker
echo "your_hub_password" >${HOME}/.docker/hub_token
docker container run -i --rm --net registry \
  -e "HUB_USER" \
  -v "${HOME}/.docker/hub_token:/var/run/secrets/hub_token:ro" \
  -v "$(pwd)/regbot.yml:/home/appuser/regbot.yml" \
  ghcr.io/regclient/regbot:latest -c /home/appuser/regbot.yml once --dry-run
```

## Run the Scripts Now

Repeat the above, but without the "dry-run" option to actually copy and delete
images. Note that this command will pull a number of images from Hub, but will
automatically rate limit itself if you have less than the specified pulls
remaining on your account.

```shell
docker container run -i --rm --net registry \
  -e "HUB_USER" \
  -v "${HOME}/.docker/hub_token:/var/run/secrets/hub_token:ro" \
  -v "$(pwd)/regbot.yml:/home/appuser/regbot.yml" \
  ghcr.io/regclient/regbot:latest -c /home/appuser/regbot.yml once
```

## Run the Scripts on a Schedule

If the "once" mode is successful, you can run the server in the background to
constantly maintain the local registry using the defined scripts.

```shell
docker container run -d --restart=unless-stopped --name regbot --net registry \
  -e "HUB_USER" \
  -v "${HOME}/.docker/hub_token:/var/run/secrets/hub_token:ro" \
  -v "$(pwd)/regbot.yml:/home/appuser/regbot.yml" \
  ghcr.io/regclient/regbot:latest -c /home/appuser/regbot.yml server -v debug
```

## Inspect using regctl

```shell
./regctl repo ls localhost:5000
```

The repositories we've created should be visible.

```shell
tag_format='{{$name := .Name}}{{range .Tags}}{{printf "%s:%s\n" $name .}}{{end}}'
./regctl tag ls localhost:5000/library/alpine --format "${tag_format}" | sort && \
./regctl tag ls localhost:5000/library/debian --format "${tag_format}" | sort && \
./regctl tag ls localhost:5000/regclient/example --format "${tag_format}" | sort
```

That should show the tags in each of the repositories, without the old example
ci tags that were pruned, while leaving the other example tags.
