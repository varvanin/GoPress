package version

var Current = "0.2.0-dev"

// Release builds set the public trust anchor; private signing keys never enter images.
var UpdatePublicKey = ""
var UpdateFeed = "https://github.com/varvanin/GoPress/releases/latest/download/update.json"
var ImageRepository = "ghcr.io/varvanin/gopress"

var Deployment = "binary"
