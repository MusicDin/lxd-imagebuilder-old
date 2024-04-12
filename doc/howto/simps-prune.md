# How to prune images hosted on Simplestream server

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

Prune command is used to remove no longer needed product versions (images).
Once prunning is complete, product catalog and simplestream index are updated accordingly.

## Retention policy

Product versions are retrieved from existing product catalog and removed according to the set
retention policy.

Flag `--retain-builds` instructs simplestream-maintainer to keep latest *n* versions (sorted
alphabetically) and remove evertyhing else.

Flag `--retain-days` sets the maximum age of the product version and ensures that no product version
older then the specified number of days remains on the system or product catalog.
By default, this flag is set to `0` which means the product versions are not pruned by age.

## Dangling images

By default, product versions are retrieved from product catalog, which means there may be exist an
incomplete version that was not included in the catalog but is no longer required.

Flag `--dangling` instructs simplestream-maintainer to remove product versions that are not
referenced by the product catalog. Unreferenced versions are removed only if they are older then 6
hours, to prevent accidental removal of freshly uploaded or generated images that are not yet
included in the product catalog.


