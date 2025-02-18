package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	gokit_log "github.com/go-kit/kit/log"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/alerting"
	old_notifiers "github.com/grafana/grafana/pkg/services/alerting/notifiers"
	"github.com/grafana/grafana/pkg/setting"
)

type DiscordNotifier struct {
	old_notifiers.NotifierBase
	log        log.Logger
	tmpl       *template.Template
	Content    string
	AvatarURL  string
	WebhookURL string
}

func NewDiscordNotifier(model *NotificationChannelConfig, t *template.Template) (*DiscordNotifier, error) {
	if model.Settings == nil {
		return nil, alerting.ValidationError{Reason: "No Settings Supplied"}
	}

	avatarURL := model.Settings.Get("avatar_url").MustString()

	discordURL := model.Settings.Get("url").MustString()
	if discordURL == "" {
		return nil, alerting.ValidationError{Reason: "Could not find webhook url property in settings"}
	}

	content := model.Settings.Get("message").MustString(`{{ template "default.message" . }}`)

	return &DiscordNotifier{
		NotifierBase: old_notifiers.NewNotifierBase(&models.AlertNotification{
			Uid:                   model.UID,
			Name:                  model.Name,
			Type:                  model.Type,
			DisableResolveMessage: model.DisableResolveMessage,
			Settings:              model.Settings,
			SecureSettings:        model.SecureSettings,
		}),
		Content:    content,
		AvatarURL:  avatarURL,
		WebhookURL: discordURL,
		log:        log.New("alerting.notifier.discord"),
		tmpl:       t,
	}, nil
}

func (d DiscordNotifier) Notify(ctx context.Context, as ...*types.Alert) (bool, error) {
	data := notify.GetTemplateData(ctx, d.tmpl, as, gokit_log.NewNopLogger())
	alerts := types.Alerts(as...)

	bodyJSON := simplejson.New()
	bodyJSON.Set("username", "Grafana")

	var tmplErr error
	tmpl := notify.TmplText(d.tmpl, data, &tmplErr)
	if d.Content != "" {
		bodyJSON.Set("content", tmpl(d.Content))
	}

	if d.AvatarURL != "" {
		bodyJSON.Set("avatar_url", tmpl(d.AvatarURL))
	}

	footer := map[string]interface{}{
		"text":     "Grafana v" + setting.BuildVersion,
		"icon_url": "https://grafana.com/assets/img/fav32.png",
	}

	embed := simplejson.New()
	embed.Set("title", tmpl(`{{ template "default.title" . }}`))
	embed.Set("footer", footer)
	embed.Set("type", "rich")

	color, _ := strconv.ParseInt(strings.TrimLeft(getAlertStatusColor(alerts.Status()), "#"), 16, 0)
	embed.Set("color", color)

	ruleURL, err := joinUrlPath(d.tmpl.ExternalURL.String(), "/alerting/list")
	if err != nil {
		return false, err
	}
	embed.Set("url", ruleURL)

	bodyJSON.Set("embeds", []interface{}{embed})

	if tmplErr != nil {
		return false, fmt.Errorf("failed to template discord message: %w", tmplErr)
	}

	body, err := json.Marshal(bodyJSON)
	if err != nil {
		return false, err
	}
	cmd := &models.SendWebhookSync{
		Url:         d.WebhookURL,
		HttpMethod:  "POST",
		ContentType: "application/json",
		Body:        string(body),
	}

	if err := bus.DispatchCtx(ctx, cmd); err != nil {
		d.log.Error("Failed to send notification to Discord", "error", err)
		return false, err
	}
	return true, nil
}

func (d DiscordNotifier) SendResolved() bool {
	return !d.GetDisableResolveMessage()
}
