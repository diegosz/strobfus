# DEV

## Fork Motivation

The pre-fork version of this package the Parser does not allow to call Parse() a second time, it fails with an error.

For the way we use this package, we need to Parse one time to create the help template with the default values of the config struct prior the struct is filled with the values defined in the environment variables, and then we need to Parse a second time after filling the environment variable values so the flag values overwrite any environment defined or default values.

> Note that if we call ShowHelp for the first time after the second Parse, the default values are affected by the environment variables. To avoid this we need to call ShowHelp prior the second Parser or better Help.ExtractValues to capture the real default values.

## TODO

* [ ] Think if it's worthwhile to do an upstream pull request..., the original package is awesome as it is, may me this complicate things for the rest...

## Chore

In the local clone, after pulling commits and tags from upstream and reapplying the fork changes and testing, we need to apply a tag that is `semver` superior but also that is not going to have conflicts with any future upstream tag.

Use tags with the following structure:

```text
AAA = 3 digit 0 padded UPSTREAM_MAYOR

BBB = 3 digit 0 padded UPSTREAM_MINOR

CCC = 3 digit 0 padded UPSTREAM_PATCH

DDD = 3 digit 0 padded incremental counter starting in 1

FORK_TAG = <UPSTREAM_PATCH><AAA><BBB><CCC><DDD>

v<UPSTREAM_MAYOR>.<UPSTREAM_MINOR>.<FORK_TAG>

v<UPSTREAM_MAYOR>.<UPSTREAM_MINOR>.<FORK_TAG>-pre

v<UPSTREAM_MAYOR>.<UPSTREAM_MINOR>.<FORK_TAG>+build

v<UPSTREAM_MAYOR>.<UPSTREAM_MINOR>.<FORK_TAG>-pre+build
```

For example:

| upstream | fork              | issue |
| -------- | ----------------- | ----- |
| 2.3.1    |                   |       |
|          | 2.3.1002003001001 | ok    |
| 2.3.2    |                   |       |
|          | 2.3.2002003002001 | ok    |
|          | 2.3.2002003002002 | ok    |
|          | 2.3.2002003002003 | ok    |
| 2.4.1    |                   |       |
|          | 2.4.1002004001001 | ok    |
| 3.3.1    |                   |       |
|          | 3.3.1003003001001 | ok    |

Invalid options:

| upstream | fork      | issue                                                 |
| -------- | --------- | ----------------------------------------------------- |
| 2.3.1    |           |                                                       |
|          | 2.3.2     | future conflict with upstream tag                     |
|          | 2.4.1     | worst future conflict with upstream tag               |
|          | 3.3.1     | even worst future conflict with upstream tag          |
|          | 2.3.1-001 | is a semver pre-release so it's not picked for update |
|          | 2.3.1+001 | the build meta tag it's not picked for update         |
| 2.3.2    |           |                                                       |
| 2.4.1    |           |                                                       |
| 3.3.1    |           |                                                       |
