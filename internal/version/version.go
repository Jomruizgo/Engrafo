package version

// Current is the canonical engrafo version.
// Update this when cutting a release; plugin.json must match.
const Current = "2.3.0"

// EngramCompatible is the pinned engram version that engrafo has been tested against.
// engrafo installs this version automatically if engram is absent or older.
// Users who upgrade engram beyond this version do so at their own risk.
const EngramCompatible = "v1.16.3"
