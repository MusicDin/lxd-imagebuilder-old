# Simplestream directory structure

`simplestream-maintainer` is a CLI tool for building simplestream product catalog and removing expired simplestream product versions.

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
