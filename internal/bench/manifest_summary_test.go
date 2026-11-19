// RenderManifestSummary: tabla directa y pura (vacío frente a lleno,
// un caso por cada if y cada for). Sin IO ni fixtures: la función no
// toca disco ni reloj, así que cada rama se ejerce con un HostManifest
// literal y aserciones de contiene / no-contiene más un caso exacto
// que fija el formato byte a byte.
package bench

import (
	"strings"
	"testing"
)

func fullSummaryManifest() HostManifest {
	return HostManifest{
		Host: "opencode", Version: "v2.2.5", Class: ClassJourney,
		RequiresCredentials: false, RepeatableInCI: true,
		ProbeCommands:   []string{"opencode --version"},
		JourneyCommands: []string{"regulus bench host-journeys --run --host opencode"},
		AgentCommands:   []string{"opencode run --model <model>"},
		Degradation:     "sin degradación",
		JourneyEvidence: "journey green 2026-11-16", JourneyDate: "2026-11-16",
		ProbeNote: "probe 2026-11-17: ok",
		TraceHint: "regulus trace timeline --run x --rebuild",
	}
}

func TestRenderManifestSummaryTable(t *testing.T) {
	cases := []struct {
		name   string
		m      HostManifest
		want   []string
		absent []string
	}{
		{
			name: "vacío",
			m:    HostManifest{},
			want: []string{
				"==  ==\n",
				"versión: \n",
				"clase: \n",
				"credenciales: false | repetible en CI: false\n",
				"degradación: \n",
			},
			absent: []string{
				"evidencia journey:", "nota probe:", "traza: ",
				"\nprobe: ", "\njourney: ", "\nagent: ",
			},
		},
		{
			name: "lleno",
			m:    fullSummaryManifest(),
			want: []string{
				"== opencode ==\n",
				"versión: v2.2.5\n",
				"clase: journey\n",
				"credenciales: false | repetible en CI: true\n",
				"evidencia journey: journey green 2026-11-16 (2026-11-16)\n",
				"nota probe: probe 2026-11-17: ok\n",
				"degradación: sin degradación\n",
				"probe: opencode --version\n",
				"journey: regulus bench host-journeys --run --host opencode\n",
				"agent: opencode run --model <model>\n",
				"traza: regulus trace timeline --run x --rebuild\n",
			},
		},
		{
			name: "credenciales y CI",
			m:    HostManifest{Host: "c", RequiresCredentials: true, RepeatableInCI: true},
			want: []string{"credenciales: true | repetible en CI: true\n"},
		},
		{
			name: "solo evidencia journey",
			m:    HostManifest{Host: "h", JourneyEvidence: "ev", JourneyDate: "2026-11-16"},
			want: []string{"evidencia journey: ev (2026-11-16)\n"},
			absent: []string{
				"nota probe:", "traza: ",
				"\nprobe: ", "\njourney: ", "\nagent: ",
			},
		},
		{
			name: "fecha sin detalle",
			m:    HostManifest{Host: "h", JourneyDate: "2026-11-16"},
			want: []string{"evidencia journey:  (2026-11-16)\n"},
		},
		{
			name: "solo nota probe",
			m:    HostManifest{Host: "h", ProbeNote: "solo probe"},
			want: []string{"nota probe: solo probe\n"},
			absent: []string{
				"evidencia journey:", "traza: ",
				"\nprobe: ", "\njourney: ", "\nagent: ",
			},
		},
		{
			name:   "solo traza",
			m:      HostManifest{Host: "h", TraceHint: "t"},
			want:   []string{"traza: t\n"},
			absent: []string{"evidencia journey:", "nota probe:"},
		},
		{
			name:   "solo comandos probe",
			m:      HostManifest{Host: "h", ProbeCommands: []string{"p1", "p2"}},
			want:   []string{"probe: p1\n", "probe: p2\n"},
			absent: []string{"\njourney: ", "\nagent: "},
		},
		{
			name:   "solo comandos journey",
			m:      HostManifest{Host: "h", JourneyCommands: []string{"j1"}},
			want:   []string{"journey: j1\n"},
			absent: []string{"\nprobe: ", "\nagent: "},
		},
		{
			name:   "solo comandos agent",
			m:      HostManifest{Host: "h", AgentCommands: []string{"a1"}},
			want:   []string{"agent: a1\n"},
			absent: []string{"\nprobe: ", "\njourney: "},
		},
		{
			name: "degradación",
			m:    HostManifest{Host: "h", Degradation: "d"},
			want: []string{"degradación: d\n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderManifestSummary(tc.m)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("falta %q en:\n%s", w, got)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("sobra %q en:\n%s", a, got)
				}
			}
		})
	}
}

func TestRenderManifestSummaryFullExact(t *testing.T) {
	want := "== opencode ==\n" +
		"versión: v2.2.5\n" +
		"clase: journey\n" +
		"credenciales: false | repetible en CI: true\n" +
		"evidencia journey: journey green 2026-11-16 (2026-11-16)\n" +
		"nota probe: probe 2026-11-17: ok\n" +
		"degradación: sin degradación\n" +
		"probe: opencode --version\n" +
		"journey: regulus bench host-journeys --run --host opencode\n" +
		"agent: opencode run --model <model>\n" +
		"traza: regulus trace timeline --run x --rebuild\n"
	if got := RenderManifestSummary(fullSummaryManifest()); got != want {
		t.Fatalf("salida exacta:\n got %q\nwant %q", got, want)
	}
}
