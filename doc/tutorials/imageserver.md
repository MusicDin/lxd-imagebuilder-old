# Use `simplestream-maintainer` to automate image server maintenance

Simplestream maintainer is capable of generating Simplestream product catalog and removing expired
or invalid product versions. However, it is a simple CLI tool that has to be invoked every time an
action needs to done.

To automate process of maintaining simplestream image server, we recommend triggering build and
prune commands periodically, either via cronjobs or systemd units.

On servers that host hundreds or more images, build process can take quite some long time because
ithas to calculate missing hashes and generate missing delta files. In such cases, we recommend
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
