# Simplestream configuration

Product version can contain an optional `images.yaml` configuration file.
This file can contain additional image information, such as release aliases or image requirements
which cannot be parsed purely from the directory structure.

All simplestream related configuration is located within a `simplestream` filed, which currently
supports:
- `distro_name` - Pretty name of the distribution that is shown in when listing images in LXD.
  It defaults to the distribution name parsed from directory structure.
- `release_aliases` - A map of distribution release and a comma delimited string of release aliases.
- `requirements` - A list of image requirements with optional filters.

Note: Configuration file is always parsed from the last product version (alphabetically sorted).

Example for distro name:
```yaml
simplestream:
  distro_name: Ubuntu Core
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
