# Yop CLI Distribution

This context defines the identities and states used to distribute Yop CLI and its bundled Skills safely.

## Language

**Declared Version**:
The single version identity approved in source before a release begins.
_Avoid_: Build version, tag version

**Release Candidate**:
The immutable, checksummed set of CLI archives and package metadata produced from one validated source revision.
_Avoid_: Build output, draft artifacts

**Distribution Channel**:
The ordered release stream a version belongs to: Stable or Beta.
_Avoid_: Environment, branch

**Runtime Update**:
A transition from an installed Declared Version to a newer version in the same Distribution Channel, including the matching Skills revision.
_Avoid_: Reinstall, upgrade check
