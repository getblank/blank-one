package internet

import (
	"fmt"
	"testing"
)

func TestConvertHookURI(t *testing.T) {
	testData := []struct {
		in  string
		res string
	}{
		{
			"/some/path/to/:id",
			"/some/path/to/{id}",
		},
		{
			"/some/path/with/:id/and/other",
			"/some/path/with/{id}/and/other",
		},
		{
			"/some/path/with/:id/and/another/:id",
			"/some/path/with/{id}/and/another/{id}",
		},
		{
			"/some/path/with/:id/and/another/:param",
			"/some/path/with/{id}/and/another/{param}",
		},
		{
			"/simple/path",
			"/simple/path",
		},
	}

	for i, v := range testData {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			res := convertHookURI(v.in)
			if res != v.res {
				t.Fatalf("convertHookURI(%s) => %s, expected: %s", v.in, res, v.res)
			}
		})
	}
}
