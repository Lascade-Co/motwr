package captions

import "testing"

func w(text string, start, end float64) Word { return Word{Text: text, Start: start, End: end} }

func TestBuildPagesGroupsWithinWindow(t *testing.T) {
	words := []Word{
		w("Our", 0.0, 0.3), w("journey", 0.35, 0.8), w("begins", 0.85, 1.1),
		w("in", 1.3, 1.4), w("Kochi", 1.5, 2.0),
	}
	pages := BuildPages(words, 1.2)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if len(pages[0].Words) != 3 || len(pages[1].Words) != 2 {
		t.Fatalf("page sizes %d/%d, want 3/2", len(pages[0].Words), len(pages[1].Words))
	}
	if pages[1].Start() != 1.3 {
		t.Errorf("page 2 start %.2f, want 1.30", pages[1].Start())
	}
}

func TestBuildPagesSingleWordPages(t *testing.T) {
	words := []Word{w("one", 0, 0.5), w("two", 2, 2.5), w("three", 4, 4.5)}
	pages := BuildPages(words, 1.2)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
}

func TestBuildPagesEmpty(t *testing.T) {
	if pages := BuildPages(nil, 1.2); len(pages) != 0 {
		t.Fatalf("expected no pages, got %d", len(pages))
	}
}
