# sr

[![CI](https://github.com/winebarrel/sr/actions/workflows/ci.yml/badge.svg)](https://github.com/winebarrel/sr/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/winebarrel/sr/branch/main/graph/badge.svg)](https://codecov.io/gh/winebarrel/sr)
[![AI Generated](https://img.shields.io/badge/AI%20Generated-Claude-orange?logo=anthropic)](https://claude.ai/claude-code)

Run a command against a different IAM Identity Center permission set.

A profile in `~/.aws/config` pins exactly one `sso_role_name`, so reaching a
second permission set in the same account means editing that file and
remembering to put it back. The edit is global and outlives the command it was
made for, which is how a `terraform apply` ends up running as an administrator
nobody meant to be.

`sr` leaves the file alone. The profile still supplies the account, the start
URL and the region; only the permission set is substituted, and only for the
one command you run.

## Installation

Download an archive for your platform from the
[releases page](https://github.com/winebarrel/sr/releases) and put the `sr`
binary somewhere on your `PATH`:

```
tar xzf sr_Darwin_arm64.tar.gz
install sr /usr/local/bin/
```

Or build it yourself with Go 1.27 or later:

```
go install github.com/winebarrel/sr/cmd/sr@latest
```

macOS and Linux only: `sr` replaces itself with the command it runs, which has
no equivalent on Windows.

## Usage

```
Usage: sr --role=STRING <command> ... [flags]

Arguments:
  <command> ...    Command to run.

Flags:
  -h, --help                   Show context-sensitive help.
      --version
  -p, --profile=STRING         Profile to take the account, start URL and region
                               from ($AWS_PROFILE).
  -r, --role=STRING            Permission set to assume, in full or as an alias.
  -a, --alias=KEY=VALUE,...    Short names for permission sets, as
                               'short=PermissionSetName' ($SR_ALIAS).
```

```
$ sr -p example -r ReadOnlyAccess terraform plan
```

`-r` is required: there is no permission set to fall back on, since taking the
one the profile names would run the command as whatever you were trying to
avoid.

Everything from the command onwards belongs to it, flags included, so nothing
needs escaping:

```
$ sr -p example -r ReadOnlyAccess aws s3 ls --region us-east-1
```

A `--` before the command is accepted if you are in the habit of writing one.

Without `-p`, the profile comes from `AWS_PROFILE` as usual:

```
$ AWS_PROFILE=example sr -r ReadOnlyAccess terraform plan
```

### Aliases

Permission set names are long and repetitive to type. `SR_ALIAS` gives them
short names:

```sh
export SR_ALIAS='ro=ReadOnlyAccess,admin=AdministratorAccess,po=PowerUserAccess'
```

```
$ sr -p example -r ro terraform plan
```

The expansion is purely local — a name is either an alias you defined or the
permission set name itself. `sr` never asks Identity Center what exists, so
there is no lookup to wait for and no partial matching to be surprised by.

## What it does

1. Loads the profile the way any other AWS tool loads it, with the permission
   set you named substituted for its `sso_role_name`.
2. Retrieves credentials for that permission set, using the SSO access token
   the AWS CLI cached under `~/.aws/sso/cache`.
3. Replaces itself with your command, with the credentials in its environment.

`GetRoleCredentials` is the only call it makes, and it makes no attempt to sign
you in: if there is no cached token, or it has expired, it says so and stops.

```
$ sr -p example -r ro terraform plan
sr: error: the SSO session has expired or is invalid: run `aws sso login`
```

The command runs with `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
`AWS_SESSION_TOKEN`, `AWS_CREDENTIAL_EXPIRATION` and the region set, and with
`AWS_PROFILE` removed so that nothing sends it back to the permission set the
profile names.
