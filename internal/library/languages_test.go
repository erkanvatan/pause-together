package library

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestLanguagesStartEmpty(t *testing.T) {
	got, err := newTestLibraries(t).GetLanguages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Languages{Audio: "", Subtitles: []string{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSetLanguages(t *testing.T) {
	tests := []struct {
		name string
		in   Languages
		want Languages
		err  error
	}{
		{"normalized", Languages{Audio: "ENG", Subtitles: []string{"tur", " en "}},
			Languages{Audio: "en", Subtitles: []string{"tr", "en"}}, nil},
		{"duplicates dropped", Languages{Subtitles: []string{"eng", "tur", "en", "tr"}},
			Languages{Subtitles: []string{"en", "tr"}}, nil},
		{"original and none", Languages{Subtitles: nil}, Languages{Subtitles: []string{}}, nil},
		{"bad audio", Languages{Audio: "english"}, Languages{}, ErrBadLang},
		{"bad subtitle", Languages{Subtitles: []string{"tr", "x"}}, Languages{}, ErrBadLang},
		{"empty subtitle", Languages{Subtitles: []string{""}}, Languages{}, ErrBadLang},
		{"too many", Languages{Subtitles: []string{"en", "tr", "de", "fr", "es", "it", "pt", "nl", "sv", "da", "fi"}},
			Languages{}, ErrBadLang},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			l := newTestLibraries(t)
			got, err := l.SetLanguages(ctx, tt.in)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("returned %+v, want %+v", got, tt.want)
			}
			saved, err := l.GetLanguages(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(saved, tt.want) {
				t.Errorf("saved %+v, want %+v", saved, tt.want)
			}
		})
	}
}

func TestSetLanguagesKeepsOldOnError(t *testing.T) {
	ctx := context.Background()
	l := newTestLibraries(t)
	old := Languages{Audio: "tr", Subtitles: []string{"en"}}
	if _, err := l.SetLanguages(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetLanguages(ctx, Languages{Audio: "nope"}); !errors.Is(err, ErrBadLang) {
		t.Fatalf("err = %v, want ErrBadLang", err)
	}
	got, err := l.GetLanguages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, old) {
		t.Errorf("got %+v, want %+v", got, old)
	}
}
