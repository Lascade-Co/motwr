package captions

// Word is one TTS word timestamp in seconds.
type Word struct {
	Text       string
	Start, End float64
}

// Page is a group of consecutive words displayed together (CONTEXT.md
// "Caption Page").
type Page struct {
	Words []Word
}

func (p Page) Start() float64 { return p.Words[0].Start }

// BuildPages groups words TikTok-style: a word joins the current page while
// it starts within maxPageDur of the page start.
func BuildPages(words []Word, maxPageDur float64) []Page {
	var pages []Page
	for _, w := range words {
		if len(pages) == 0 || w.Start-pages[len(pages)-1].Start() >= maxPageDur {
			pages = append(pages, Page{})
		}
		p := &pages[len(pages)-1]
		p.Words = append(p.Words, w)
	}
	return pages
}
