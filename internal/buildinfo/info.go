package buildinfo

// Release builds override these values with -ldflags. Development builds can
// check releases but cannot install updates without a pinned signing key.
var Version = "0.1.3"
var UpdatePublicKey = ""
var Commit = "development"

const Repository = "al6nlee/tunnelix"
