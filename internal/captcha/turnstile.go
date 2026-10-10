package captcha

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// Turnstile Cloudflare Turnstile
// https://developers.cloudflare.com/turnstile/get-started/server-side-validation/
type Turnstile struct {
	SiteKey, Secret string
}

func (t *Turnstile) Name() string { return "turnstile" }

func (t *Turnstile) Widget() template.HTML {
	return template.HTML(fmt.Sprintf(
		`<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>`+
			`<div class="cf-turnstile" data-sitekey="%s" data-callback="onCaptchaToken"></div>`,
		html.EscapeString(t.SiteKey)))
}

func (t *Turnstile) Verify(ctx context.Context, token, remoteIP string) (bool, error) {
	form := url.Values{"secret": {t.Secret}, "response": {token}, "remoteip": {remoteIP}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return siteverify(req)
}
