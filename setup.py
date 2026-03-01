# This file used to power the Python/py2app build pipeline.  The
# project has since migrated to a pure-Go macOS bundle, and `make build-app`
# handles packaging.  The old workflow is kept around only for historical
# reference; attempting to run it will raise an explicit error so that
# developers don't accidentally use it.

raise RuntimeError(
    "py2app build has been deprecated. Please use `make build-app` instead."
)
