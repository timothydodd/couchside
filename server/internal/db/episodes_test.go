package db

import (
	"context"
	"testing"
)

func TestEpisodeDetailsAndCredits(t *testing.T) {
	d := openTest(t)
	bg := context.Background()
	lib, _ := d.CreateLibrary(bg, "TV", "/tv", "tv")
	show, _, _ := d.EnsureItem(bg, lib, "series", "Ghosts", 2021)
	var eps []int64
	for i := 1; i <= 3; i++ {
		ep, _ := d.EnsureEpisode(bg, show, 1, i, "", "")
		if _, err := d.UpsertFile(bg, File{LibraryID: lib, MediaItemID: show, EpisodeID: &ep, Path: "/tv/Ghosts/S01E0" + string(rune('0'+i)) + ".mkv", Size: 1, Mtime: 1}, 1); err != nil {
			t.Fatal(err)
		}
		eps = append(eps, ep)
	}
	rt := 22
	if err := d.ApplyEpisodeMeta(bg, show, []EpisodeMeta{
		{Season: 1, Episode: 2, Title: "Hello!", Released: "2021-10-14", Plot: "Sam meets the ghosts.", RuntimeMin: &rt, StillURL: "https://image.tmdb.org/t/p/w780/s2.jpg", TMDBID: 42,
			Credits: []Credit{{PersonID: 7, Name: "Guest Star", ProfilePath: "/g.jpg", Kind: "cast", Role: "Neighbour"}, {PersonID: 8, Name: "Trent O'Donnell", Kind: "crew", Role: "Director", Order: 100}}},
		{Season: 1, Episode: 9, Title: "Not ours"}, // no file: ignored
	}); err != nil {
		t.Fatal(err)
	}
	e, err := d.Episode(bg, eps[1])
	if err != nil || e.Title != "Hello!" || e.Plot != "Sam meets the ghosts." || e.RuntimeMin == nil || *e.RuntimeMin != 22 || e.TMDBID != 42 || e.SeriesID != show {
		t.Fatalf("episode = %+v, %v", e, err)
	}
	cast, crew, err := d.EpisodeCredits(bg, eps[1])
	if err != nil || len(cast) != 1 || cast[0].Role != "Neighbour" || !cast[0].HasPhoto || len(crew) != 1 || crew[0].Role != "Director" {
		t.Fatalf("credits = %+v / %+v, %v", cast, crew, err)
	}
	prev, next, err := d.EpisodeNeighbours(bg, e)
	if err != nil || prev == nil || prev.Episode != 1 || next == nil || next.Episode != 3 {
		t.Fatalf("neighbours = %+v, %+v, %v", prev, next, err)
	}
	if first, _ := d.Episode(bg, eps[0]); true {
		if p, n, _ := d.EpisodeNeighbours(bg, first); p != nil || n == nil || n.Episode != 2 {
			t.Errorf("first episode's neighbours = %+v, %+v", p, n)
		}
	}
	stills, err := d.EpisodeStills(bg, show)
	if err != nil || len(stills) != 1 || stills[0].StillURL == "" {
		t.Fatalf("stills = %+v, %v", stills, err)
	}
	pe, err := d.PersonEpisodes(bg, 7)
	if err != nil || len(pe) != 1 || pe[0].SeriesTitle != "Ghosts" || pe[0].Roles[0] != "Neighbour" {
		t.Fatalf("person episodes = %+v, %v", pe, err)
	}
	// A provider without credits (OMDb) keeps TMDB's.
	if err := d.ApplyEpisodeMeta(bg, show, []EpisodeMeta{{Season: 1, Episode: 2, Title: "Hello!"}}); err != nil {
		t.Fatal(err)
	}
	if cast, _, _ := d.EpisodeCredits(bg, eps[1]); len(cast) != 1 {
		t.Errorf("credits were wiped by a provider without any")
	}
	// People credited only on episodes survive pruning.
	if gone, err := d.PrunePeople(bg); err != nil || len(gone) != 0 {
		t.Fatalf("prune = %v, %v", gone, err)
	}
	if _, err := d.Person(bg, 7); err != nil {
		t.Errorf("guest star was pruned: %v", err)
	}
}
