# How to build Simplestream index

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

Final product catalog is generated in `streams/<stream_version>/<stream>.json` and index file in
`streams/<stream_version>/index.json`.

## Checksum verification

If a specific version contains `SHA256SUMS` file, checksums are parsed from it, and compared against
the calculated file hashes. If there is a mismatch, the version is not included in the final product
catalog.

This allows verification of images that are built on the remote location and pushed to the
simplestream server.

## Webpage

Build command allows to optionally generate a static webpage (index.html) in stream's root directory.
Webpage contains a table of all products that are extracted from the final product catalog.
