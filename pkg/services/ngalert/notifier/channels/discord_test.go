package channels

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/alerting"
)

func TestDiscordNotifier(t *testing.T) {
	tmpl := templateForTests(t)

	externalURL, err := url.Parse("http://localhost")
	require.NoError(t, err)
	tmpl.ExternalURL = externalURL

	cases := []struct {
		name         string
		settings     string
		alerts       []*types.Alert
		expMsg       map[string]interface{}
		expInitError error
		expMsgError  error
	}{
		{
			name:     "Default config with one alert",
			settings: `{"url": "http://localhost"}`,
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "alert1", "lbl1": "val1"},
						Annotations: model.LabelSet{"ann1": "annv1"},
					},
				},
			},
			expMsg: map[string]interface{}{
				"content": "\n**Firing**\nLabels:\n - alertname = alert1\n - lbl1 = val1\nAnnotations:\n - ann1 = annv1\nSource: \n\n\n\n\n",
				"embeds": []interface{}{map[string]interface{}{
					"color": 1.4037554e+07,
					"footer": map[string]interface{}{
						"icon_url": "https://grafana.com/assets/img/fav32.png",
						"text":     "Grafana v",
					},
					"title": "[FIRING:1]  (val1)",
					"url":   "http://localhost/alerting/list",
					"type":  "rich",
				}},
				"username": "Grafana",
			},
			expInitError: nil,
			expMsgError:  nil,
		},
		{
			name: "Custom config with multiple alerts",
			settings: `{
				"avatar_url": "https://grafana.com/assets/img/fav32.png",
				"url": "http://localhost",
				"message": "{{ len .Alerts.Firing }} alerts are firing, {{ len .Alerts.Resolved }} are resolved"
			}`,
			alerts: []*types.Alert{
				{
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "alert1", "lbl1": "val1"},
						Annotations: model.LabelSet{"ann1": "annv1"},
					},
				}, {
					Alert: model.Alert{
						Labels:      model.LabelSet{"alertname": "alert1", "lbl1": "val2"},
						Annotations: model.LabelSet{"ann1": "annv2"},
					},
				},
			},
			expMsg: map[string]interface{}{
				"avatar_url": "https://grafana.com/assets/img/fav32.png",
				"content":    "2 alerts are firing, 0 are resolved",
				"embeds": []interface{}{map[string]interface{}{
					"color": 1.4037554e+07,
					"footer": map[string]interface{}{
						"icon_url": "https://grafana.com/assets/img/fav32.png",
						"text":     "Grafana v",
					},
					"title": "[FIRING:2]  ",
					"url":   "http://localhost/alerting/list",
					"type":  "rich",
				}},
				"username": "Grafana",
			},
			expInitError: nil,
			expMsgError:  nil,
		},
		{
			name:         "Error in initialization",
			settings:     `{}`,
			expInitError: alerting.ValidationError{Reason: "Could not find webhook url property in settings"},
		},
		{
			name: "Error in building messsage",
			settings: `{
				"url": "http://localhost",
				"message": "{{ .Status }"
			}`,
			expMsgError: errors.New("failed to template discord message: template: :1: unexpected \"}\" in operand"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			settingsJson, err := simplejson.NewJson([]byte(c.settings))
			require.NoError(t, err)

			m := &NotificationChannelConfig{
				Name:     "discord_testing",
				Type:     "discord",
				Settings: settingsJson,
			}

			dn, err := NewDiscordNotifier(m, tmpl)
			if c.expInitError != nil {
				require.Error(t, err)
				require.Equal(t, c.expInitError.Error(), err.Error())
				return
			}
			require.NoError(t, err)

			body := ""
			bus.AddHandlerCtx("test", func(ctx context.Context, webhook *models.SendWebhookSync) error {
				body = webhook.Body
				return nil
			})

			ctx := notify.WithGroupKey(context.Background(), "alertname")
			ctx = notify.WithGroupLabels(ctx, model.LabelSet{"alertname": ""})
			ok, err := dn.Notify(ctx, c.alerts...)
			if c.expMsgError != nil {
				require.False(t, ok)
				require.Error(t, err)
				require.Equal(t, c.expMsgError.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			require.True(t, ok)

			expBody, err := json.Marshal(c.expMsg)
			require.NoError(t, err)

			require.JSONEq(t, string(expBody), body)
		})
	}
}
