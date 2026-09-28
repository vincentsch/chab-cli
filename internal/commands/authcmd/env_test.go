package authcmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vincentsch/chab-cli/internal/output"
)

func TestEnvironmentReportKeepsStableOrderedMachineShape(t *testing.T) {
	report := envReport{
		Profile:        "staging",
		BaseURL:        "https://staging.example.test",
		APIBaseURL:     "https://staging.example.test/api/v1",
		Locale:         "",
		SecretVariable: envSecretVariable,
	}
	got, err := output.StableJSONBytes(report)
	if err != nil {
		t.Fatalf("StableJSONBytes() error = %v", err)
	}
	const want = "{\n" +
		"  \"profile\": \"staging\",\n" +
		"  \"base_url\": \"https://staging.example.test\",\n" +
		"  \"api_base_url\": \"https://staging.example.test/api/v1\",\n" +
		"  \"locale\": \"\",\n" +
		"  \"secret_variable\": \"CHAB_API_KEY\"\n" +
		"}\n"
	if string(got) != want {
		t.Fatalf("JSON = %q, want %q", got, want)
	}
}

func TestEnvironmentDetailsSeparateHumanAndPlainRecords(t *testing.T) {
	for _, test := range []struct {
		name       string
		locale     string
		human      string
		plainData  string
		plainProse string
	}{
		{
			name:   "locale set",
			locale: "en",
			human: "CI authentication environment\n" +
				"Profile: staging\n" +
				"Base URL: https://staging.example.test\n" +
				"API base URL: https://staging.example.test/api/v1\n" +
				"Locale: en\n" +
				"Set CHAB_API_KEY from your CI provider's secret store.\n",
			plainData: "CHAB_PROFILE\tstaging\n" +
				"CHAB_BASE_URL\thttps://staging.example.test\n" +
				"CHAB_API_BASE_URL\thttps://staging.example.test/api/v1\n" +
				"CHAB_LOCALE\ten\n",
			plainProse: "Set CHAB_API_KEY from your CI provider's secret store.\n",
		},
		{
			name:   "locale unset",
			locale: "",
			human: "CI authentication environment\n" +
				"Profile: staging\n" +
				"Base URL: https://staging.example.test\n" +
				"API base URL: https://staging.example.test/api/v1\n" +
				"Set CHAB_API_KEY from your CI provider's secret store.\n",
			plainData: "CHAB_PROFILE\tstaging\n" +
				"CHAB_BASE_URL\thttps://staging.example.test\n" +
				"CHAB_API_BASE_URL\thttps://staging.example.test/api/v1\n",
			plainProse: "Set CHAB_API_KEY from your CI provider's secret store.\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := envReport{
				Profile:        "staging",
				BaseURL:        "https://staging.example.test",
				APIBaseURL:     "https://staging.example.test/api/v1",
				Locale:         test.locale,
				SecretVariable: envSecretVariable,
			}
			var human bytes.Buffer
			envHumanDetail(report).Render(&human)
			plainData, plainProse := output.PlainBytes(envPlainDetail(report).RenderPlain)
			if human.String() != test.human {
				t.Fatalf("human = %q, want %q", human.String(), test.human)
			}
			if string(plainData) != test.plainData || string(plainProse) != test.plainProse {
				t.Fatalf("plain data/prose = %q / %q, want %q / %q", plainData, plainProse, test.plainData, test.plainProse)
			}
			if strings.Contains(string(plainData), "CI authentication environment") {
				t.Fatalf("plain data contains human headline: %q", plainData)
			}
			if strings.Contains(human.String(), "CHAB_PROFILE") || strings.Contains(human.String(), "CHAB_BASE_URL") {
				t.Fatalf("human output contains plain keys: %q", human.String())
			}
			if strings.Contains(string(plainData), "SECRET_VARIABLE") || strings.Contains(human.String(), "SECRET_VARIABLE") {
				t.Fatalf("detail output contains forbidden SECRET_VARIABLE row")
			}
		})
	}
}
