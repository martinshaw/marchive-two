package provider

import "testing"

func TestResolveEmptyKeepsEverything(t *testing.T) {
	got := Resolve(KindWebpage, "")
	if !got.All || !got.HTML || !got.Text || !got.Screenshot || !got.MHTML || !got.Metadata || !got.Words || got.Audio {
		t.Fatalf("webpage default = %+v", got)
	}
	pod := Resolve(KindPodcast, "")
	if !pod.Audio || !pod.Metadata || pod.HTML || pod.Words {
		t.Fatalf("podcast default = %+v", pod)
	}
	trend := Resolve(KindTrending, "")
	if !trend.Text || !trend.Metadata || !trend.Words || !trend.Media || trend.HTML || trend.Screenshot {
		t.Fatalf("trending default = %+v", trend)
	}
	hn := Resolve(KindHN, "")
	if !hn.Text || !hn.Comments || !hn.Screenshot || !hn.Metadata || !hn.Words || hn.Media || hn.HTML {
		t.Fatalf("hacker news default = %+v", hn)
	}
	search := Resolve(KindGoogle, "")
	if !search.Text || !search.Screenshot || !search.Metadata || !search.Words || search.HTML {
		t.Fatalf("google default = %+v", search)
	}
	ddg := Resolve(KindDDG, "")
	if !ddg.Text || !ddg.Screenshot || !ddg.Metadata || !ddg.Words || ddg.HTML {
		t.Fatalf("duckduckgo default = %+v", ddg)
	}
}

func TestEncodeSelection(t *testing.T) {
	raw, err := EncodeSelection(KindText, false, []string{"text", "html"})
	if err != nil {
		t.Fatal(err)
	}
	got := Resolve(KindText, raw)
	if got.All || !got.Text || got.Metadata || got.HTML {
		t.Fatalf("text selection = %+v from %s", got, raw)
	}
	if _, err := EncodeSelection(KindPodcast, false, nil); err == nil {
		t.Fatal("expected an error when nothing is selected")
	}
	all, err := EncodeSelection(KindArticles, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if all != `{"all":true}` {
		t.Fatalf("all = %s", all)
	}
}
