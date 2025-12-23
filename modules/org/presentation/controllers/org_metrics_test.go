package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestOrgAPIController_instrumentAPI_UsesStableEndpointLabel(t *testing.T) {
	c := &OrgAPIController{}

	handler := c.instrumentAPI("org.test.endpoint", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequest(http.MethodGet, "/org/api/nodes/"+strings.Repeat("a", 36), nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() != "org_api_requests_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := labelsToMap(m)
			if labels["endpoint"] == "org.test.endpoint" && labels["result"] == "4xx" {
				require.NotNil(t, m.GetCounter())
				require.GreaterOrEqual(t, m.GetCounter().GetValue(), float64(1))
				found = true
				break
			}
		}
	}
	require.True(t, found, "expected metric org_api_requests_total with endpoint label")
}

func labelsToMap(m *dto.Metric) map[string]string {
	out := map[string]string{}
	for _, lp := range m.GetLabel() {
		out[lp.GetName()] = lp.GetValue()
	}
	return out
}
