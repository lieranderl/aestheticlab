// Package assets resolves source names (app.css, gallery/tortoise.jpg,
// video/nude.mp4) to the content-hashed URLs frontend/build.mjs built, as
// listed in manifest_gen.go.
package assets

// Image is a source image's WebP variants.
type Image struct {
	// Src is the largest variant's URL, for <img src>.
	Src string
	// SrcSet lists every variant with its width ("… 200w, … 400w").
	SrcSet string
	// Width and Height are the largest variant's size, for the <img>
	// attributes that keep the layout stable while it loads.
	Width, Height int
}

// URL is the hashed URL of a built file, or "" when the build has none.
func URL(name string) string {
	return fileURL(name)
}

// Lookup is a source image's variants, if the build has them.
func Lookup(name string) (Image, bool) {
	return image(name)
}

// Names lists every built file and image name, sorted, for checks over the
// whole manifest.
func Names() (files, images []string) {
	return append([]string(nil), fileNames...), append([]string(nil), imageNames...)
}
