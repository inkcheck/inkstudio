// Package slidedoc is a presentation as editable parts, the model the
// writers in platform/pptx and platform/odp share: slides of text boxes with
// paragraphs and runs, rectangles and pictures, on a background, with
// speaker notes. InkStudio fills it from the browser's layout of a rendered
// deck. A slide that is one picture, made by Picture, keeps the look exactly
// but is not editable. Every length is in CSS pixels from the slide's
// top-left corner, at 96 per inch.
package slidedoc

import "time"

// Deck is a presentation.
type Deck struct {
	Title, Author string
	// Width and Height are the slide size.
	Width, Height float64
	Slides        []Slide
	// Created defaults to now.
	Created time.Time
}

// Picture is a slide that is one picture, filling it.
func Picture(png []byte, width, height float64, title, notes string) Slide {
	return Slide{
		Title: title,
		Notes: notes,
		Items: []Item{{Kind: KindImage, X: 0, Y: 0, W: width, H: height, PNG: png, Alt: title}},
	}
}

// Color is an sRGB colour with alpha from 0 to 1.
type Color struct {
	R uint8   `json:"r"`
	G uint8   `json:"g"`
	B uint8   `json:"b"`
	A float64 `json:"a"`
}

// Slide is one slide.
type Slide struct {
	// Background is the slide's colour, or nil for none.
	Background *Color `json:"background"`
	// BackgroundCap, when set, names a capture that paints the background:
	// a gradient or an image.
	BackgroundCap int    `json:"backgroundCap"`
	Items         []Item `json:"items"`
	// BackgroundPNG is filled in from BackgroundCap.
	BackgroundPNG []byte `json:"-"`
	// Title is the slide's title, for alternative text and outlines.
	Title string `json:"-"`
	// Notes are plain text; blank lines separate paragraphs.
	Notes string `json:"-"`
}

// Item kinds.
const (
	KindRect  = "rect"
	KindText  = "text"
	KindImage = "image"
)

// Item is a shape, in paint order.
type Item struct {
	Kind       string     `json:"kind"`
	X, Y, W, H float64    `json:"-"`
	Box        [4]float64 `json:"box"` // x, y, w, h

	// Rectangles.
	Fill      *Color  `json:"fill,omitempty"`
	Line      *Color  `json:"line,omitempty"`
	LineWidth float64 `json:"lineWidth,omitempty"`
	Radius    float64 `json:"radius,omitempty"`

	// Pictures: Cap names the capture, which fills PNG.
	Cap int    `json:"cap,omitempty"`
	Alt string `json:"alt,omitempty"`
	PNG []byte `json:"-"`

	// Text. NoWrap keeps lines as they are, for code.
	NoWrap bool   `json:"noWrap,omitempty"`
	Paras  []Para `json:"paras,omitempty"`
}

// Para is a paragraph.
type Para struct {
	// Align is "l", "ctr", "r" or "just".
	Align string `json:"align"`
	// Bullet is "" for none, a character such as "•", or "1." for numbers.
	Bullet string `json:"bullet,omitempty"`
	// Start is the first number of a numbered list.
	Start int `json:"start,omitempty"`
	// Indent is the gap from the box's left to the text, where the bullet
	// sits.
	Indent float64 `json:"indent,omitempty"`
	// Before is the space above the paragraph, after the one before it.
	Before     float64 `json:"before,omitempty"`
	LineHeight float64 `json:"lineHeight"`
	Runs       []Run   `json:"runs"`
}

// Run is text in one style, or a line break.
type Run struct {
	Text      string  `json:"text"`
	Break     bool    `json:"break,omitempty"`
	Font      string  `json:"font"`
	Mono      bool    `json:"mono,omitempty"`
	Size      float64 `json:"size"`
	Bold      bool    `json:"bold,omitempty"`
	Italic    bool    `json:"italic,omitempty"`
	Underline bool    `json:"underline,omitempty"`
	Strike    bool    `json:"strike,omitempty"`
	Color     *Color  `json:"color,omitempty"`
	// Spacing is extra space between letters.
	Spacing float64 `json:"spacing,omitempty"`
	Href    string  `json:"href,omitempty"`
}

// Normalize copies Box into X, Y, W and H, after decoding.
func (s *Slide) Normalize() {
	for i := range s.Items {
		it := &s.Items[i]
		it.X, it.Y, it.W, it.H = it.Box[0], it.Box[1], it.Box[2], it.Box[3]
	}
}
