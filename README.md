# backup
Backup for friends and family of IT experts.

## Auto start

When executed, the program configures itself to launch automatically at user login on Linux, macOS, and Windows.

## Configuration

Configuration for the restic repository and password is loaded in the
following order:

1. Environment variables `RESTIC-REPO` and `RESTIC-REPO-PASSWORD`.
2. A Pastebin document providing `restic-repo` and `restic-repo-password`.
3. Embedded defaults pointing at `~/tmp/test-backup` with a test password.

## Health check

Running the program with the `health` argument prints a detailed report about
the environment and configuration. If email or Pushover notifications are
configured, the report is sent through those channels as well.
