package model_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/lstellway/prsm/model"
)

func TestLoadResult_MarshalJSON_Pending(t *testing.T) {
	encoded, err := json.Marshal(model.Pending[int]())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(encoded); got != `{"state":"pending"}` {
		t.Errorf("got %s, want {\"state\":\"pending\"}", got)
	}
}

func TestLoadResult_MarshalJSON_Absent(t *testing.T) {
	encoded, err := json.Marshal(model.Absent[int]())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(encoded); got != `{"state":"absent"}` {
		t.Errorf("got %s, want {\"state\":\"absent\"}", got)
	}
}

func TestLoadResult_MarshalJSON_Error(t *testing.T) {
	encoded, err := json.Marshal(model.Failed[int](errors.New("boom")))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(encoded); got != `{"state":"error","error":"boom"}` {
		t.Errorf("got %s, want {\"state\":\"error\",\"error\":\"boom\"}", got)
	}
}

func TestLoadResult_MarshalJSON_LoadedZeroValue(t *testing.T) {
	// The whole point of routing Value through `any`: a Loaded zero value
	// (e.g. zero comments) must still appear in the output, not be mistaken
	// for "no value" and omitted the way a plain T field would be.
	encoded, err := json.Marshal(model.Loaded(0))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(encoded); got != `{"state":"loaded","value":0}` {
		t.Errorf("got %s, want {\"state\":\"loaded\",\"value\":0}", got)
	}
}

func TestLoadResult_MarshalJSON_LoadedStruct(t *testing.T) {
	encoded, err := json.Marshal(model.Loaded(model.CIStatus{State: model.CIStatePassing, Summary: "3/3 passed"}))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded struct {
		State string `json:"state"`
		Value struct {
			State   string `json:"State"`
			Summary string `json:"Summary"`
		} `json:"value"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.State != "loaded" || decoded.Value.State != "passing" || decoded.Value.Summary != "3/3 passed" {
		t.Errorf("got %+v", decoded)
	}
}

func TestLoadResult_MarshalJSON_EmbeddedInStruct(t *testing.T) {
	// PullRequest never sets a custom MarshalJSON of its own; this confirms
	// LoadResult's own method is enough to make a struct embedding it
	// encode correctly by default reflection, with no per-field plumbing.
	type container struct {
		CommentCount model.LoadResult[int]
	}
	encoded, err := json.Marshal(container{CommentCount: model.Pending[int]()})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(encoded); got != `{"CommentCount":{"state":"pending"}}` {
		t.Errorf("got %s", got)
	}
}
