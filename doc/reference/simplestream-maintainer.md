# Simplestream maintainer

`simplestream-maintainer` is a CLI tool capable of maintaining image server.

## Terminology

- `Stream`: Represents a directory that contains image builds.
- `Product`: Represents a unique image within a single stream. It's ID is determined by the
    directory structure as `<distro>/<release>/<arch>/<variant>`
- `ProductVersion`: Represents a version (build) of a specific image. A single product can contain 1
    or more versions. While version name can be custom, it is recommended to be soratable by time.
    Good example is a timestamp of the image build.
- `ProductCatalog`: Represents all products from a specific stream.
- `Index`: Contains a list of product catalogs and their products.

## Directory Structure

For easier representation, here is an example of the simplestream directory structure.
```
.
├── images                 // Stream name.
│   └── ubuntu             // Distribution.
│       └── noble          // Release.
│           └── amd64      // Architecture.
│               └── cloud  // Variant.
│                   ├── 20240505_1212       // Version name.
│                   │   ├── lxd.tar.xz      // Metadata file.
│                   │   ├── disk.qcow2      // Virtual machine rootfs.
│                   │   ├── rootfs.squashfs // Container rootfs.
│                   │   └── images.yaml     // Optional simplestream config.
│                   ├── 20240506_1212
│                   │   ├── lxd.tar.xz
│                   │   ├── disk.qcow2
│                   │   ├── rootfs.squashfs
│                   │   ├── disk.20240505_1212.qcow2.vcdiff // Virtual machine delta file from previous version (20240505_1212).
│                   │   ├── rootfs.20240505_1212.vcdiff     // Container delta file from previous version (20240505_1212).
│                   │   └── images.yaml
│                   └── 20240507_1212
│                       └── ...
├── images-daily
│   └── ...
├── images-minimal
│   └── ...
└── streams
    ├── v1
    │   ├── index.json
    │   ├── images.json
    │   ├── images-daily.json
    │   └── images-minimal.json
    └── v2
        └── ...
```

### Root Directory

In the above directory structure, the *simplestream's root directory* is represented with a `.` (dot).
The CLI tool expects this path to be provided as an argument for each command, as it will operate
exclusively within that directory.

### Stream directory

The *stream directory* represents the directory within the *simplestream's root directory*.
It is expected to contain built images with the following directory structure:
```
<rootDir>
└── <stream>
    └── <distro>
        └── <release>
            └── <arch>
                └── <variant>
                    └── <version>
```

For example:
```
Path:    images/ubuntu/noble/amd64/cloud/v1
---
stream:  images
distro:  ubuntu
release: noble
arch:    amd64
variant: cloud
version: v1
```

### Product Version

```
...
<version>
├── lxd.tar.xz
├── disk.qcow2
├── rootfs.squashfs
└── images.yaml
```

Each `<version>` directory is considered *complete* if it contains `lxd.tar.xz` (image metadata)
and at least one rootfs file (`*.squashfs` and/or `*.qcow2`).

In addition, hidden versions (prefixed with the dot `.<version>`) are treated as incomplete.
This allows you to first push the images to server into a hidden directory, and only once they are
fully uploaded unhide the directory. This approach prevents partially uploaded files from being
included in the product catalog.

#### Image Configuration

Configuration file `images.yaml` is optional, and holds additional image information, such as
release aliases or image requirements.
Configuration file is always parsed from the last version (alphabetically sorted).

All simplestream related configuration is located within a `simplestream` filed, which currently
supports:
- `release_aliases` - A map of distribution release and a comma delimited string of release aliases.
- `requirements` - A list of image requirements with optional filters.

If configuration file is empty or not provided at all, it will be parsed as:
```
simplestream:
  release_aliases: {}
  requirements: []
```

Example for release aliases:
```yaml
simplestream:
  release_aliases:
    jammy: 22.04     # Single alias.
    noble: 24.04,24  # Multiple aliases.
```

Example for requirements:
```yaml
simplestream:
  requirements:

  # Applied to all images (no filters).
  - requirements:
      secure_boot: false

  # Applied to images that match the filters.
  - requirements:
      nesting: true
    releases:
    - noble
    architectures:
    - amd64
    variant:
    - default
    - desktop
```

Note that requirements cannot be applied to a specific image type (`vm`, `container`).

### Index file

```
<rootDir>
├── <stream>
└── streams
    └── <streamVersion>
        ├── index.json
        └── <stream>.json
```

As depicted in the above directory structure, the CLI tool generates index and catalog files within
the *simplestream's root directory*. All files will be generated within `streams/<streamVersion>`
directory.

Index file is always named `index.json`, while catalog name is derived from the name of the stream
directory (`<stream>.json`).

## CLI

The CLI provides the following commands:
- `build` - Builds the product catalog (e.g. `images.json`) and populates the index file
  (`index.json`) accordingly.
- `prune` - Reads the product catalog and removes all product versions except the given number of
  versions to retain.

Global flags:
```
    --logformat string   Sets global log format. Valid options are text and json (default text)
    --loglevel string    Sets global log level. Valid options are debug, info, warn, and error (default info)
    --timeout uint       Timeout in seconds (default 0)
-h, --help               Prints help message for the command
-v, --version            Prints simplestream maintainer version
```

### Build

```
Usage:
  simplestream-maintainer build <path> [flags]

Flags:
      --build-webpage           Build index.html
  -d, --image-dir strings       Image directory (relative to path argument) (default [images])
      --stream-version string   Stream version (default "v1")
      --workers int             Maximum number of concurrent operations (default "<max_cpu>/2")
```

Build command is used to update the product catalog and generate a corresponding simplestream
index file. This is achieved by first reading the existing product catalog, and then traversing
through the actual directory tree of the stream to detect the differences.

Each new product version is analysed to ensure it is complete, which means the version contains all
the required files (metadata and rootfs) and is not hidden. For complete versions, the file hashes
are calculated and, if necessary, delta files are generated.

If a specific version contains `SHA256SUMS` file, checksums are parsed from it, and compared against
the calculated file hashes. If there is a mismatch, the version is not included in the final product
catalog.

Final product catalog is generated in `streams/<stream_version>/<stream>.json` and index file in
`streams/<stream_version>/index.json`.

Build command allows to optionally generate a static webpage (index.html) in stream's root directory.
Webpage contains a table of all products that are extracted from the final product catalog.


### Prune

```
Usage:
  simplestream-maintainer prune <path> [flags]

Flags:
      --dangling                Remove dangling product versions (not referenced from a product catalog)
  -d, --image-dir strings       Image directory (relative to path argument) (default [images])
      --retain-builds int       Maximum number of product versions to retain (default 10)
      --retain-days int         Maximum number of days to retain any product version
      --stream-version string   Stream version (default "v1")
```

Prune command is used to removed no longer needed product versions.

By default, it will traverse through the directory tree of the specific stream and remove oldest
versions (alphabetically sorted) in products that contain more versions than specified with the
`--retain-builds` flag (default 10). However, if any version is older then `--retain-days` it is
removed immediately. If expiry is not set (`--retain-days 0`), then product versions are kept
idefinetly.

Additionally, if `--dangling` flag is set, the simplestream-maintainer reads the existing product
catalog and finds product versions that are not referenced. Unreferenced versions are removed if
they are older then 6 hours, to prevent removing freshly uploaded images that were not yet added
to the product catalog.

Once prunning is complete, product catalog and simplestream index are updated accordingly.


## Automation

To automate process of maintaining simplestream image server, we recommend triggering build and
prune commands periodically, either via cronjobs or systemd units.

On servers that host hundreds or more images, build process can take a long time because it has
to calculate hashes for new files and generate any missing delta files. In such cases, we recommend
using systemd units to prevent triggering unnecessary builds if the previous build has not finised
yet.

Here is an example using systemd units.

> Note: Replace `<simplestream_dir>` with an your actual directory where simplestream images are
hosted and `<simplestream_user>` with your user.

```sh
# /etc/systemd/system/simplestream-maintainer.service
[Unit]
Description=Simplestream maintainer
ConditionPathIsDirectory="<simplestream_dir>/images"

[Service]
Type=oneshot
User="<simplestream_user>"
Environment=TZ=UTC

# Commands are executed in the exact same order as specified.
ExecStart=simplestream-maintainer build "<simplestream_dir>" --logformat json --loglevel warn --workers 4
ExecStart=simplestream-maintainer prune "<simplestream_dir>" --logformat json --loglevel warn --retain-builds 3 --dangling

# Processes running at "idle" level get CPU time only when no one else needs it.
# This prevents simplestream-maintainer from consuming the computational resources when
# they are used to serve the images.
CPUSchedulingPolicy=idle

# Processes running at "idle" level get I/O time only when no one else needs the disk.
# This prevents simplestream-maintainer from consuming the disk I/O when it is used to
# serve the images.
IOSchedulingClass=idle
```

And the systemd timer that triggers the above unit:
```sh
# /etc/systemd/system/simplestream-maintainer.timer
[Unit]
Description=Simplestream maintainer timer

[Timer]
OnCalendar=hourly
RandomizedDelaySec=5m
Persistent=true

[Install]
WantedBy=timers.target
```
