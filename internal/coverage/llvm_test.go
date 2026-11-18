package coverage

import (
	"encoding/json"
	"testing"
)

func Test_should_exclude_gap_segments_when_normalizing_measured_regions(t *testing.T) {
	regions, err := measuredRegions(json.RawMessage(`[[1,1,4,true,true,false],[1,5,0,true,true,true],[1,8,0,true,true,false],[1,12,0,false,false,false]]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 2 || regions[0].Count != 4 || regions[0].End.Column != 5 || regions[1].Start.Column != 8 || regions[1].Count != 0 {
		t.Fatal("gap must not become an unexecuted source region")
	}
	if _, err := measuredRegions(json.RawMessage(`[[2,1,0,true,true,false],[1,1,0,false,false,false]]`)); err == nil {
		t.Fatal("unordered source segments must be rejected")
	}
}
