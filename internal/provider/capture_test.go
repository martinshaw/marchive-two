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
