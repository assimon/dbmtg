package captcha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/http"
)

// Cap 自建的 Cap Standalone 服务
// https://github.com/tiagozip/cap
type Cap struct {
	URL, SiteKey, Secret string
}

func (c *Cap) Name() string { return "cap" }

func (c *Cap) Widget() template.HTML {
	return template.HTML(fmt.Sprintf(
		`<script src="https://cdn.jsdelivr.net/npm/cap-widget@0.1.58"></script>`+
			`<cap-widget id="cap" data-cap-api-endpoint="%s"></cap-widget>`+
			`<script>document.getElementById("cap").addEventListener("solve", (e) => onCaptchaToken(e.detail.token));</script>`,
		html.EscapeString(c.URL+"/"+c.SiteKey+"/")))
}

func (c *Cap) Verify(ctx context.Context, token, _ string) (bool, error) {
	body, _ := json.Marshal(map[string]string{"secret": c.Secret, "response": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.URL+"/"+c.SiteKey+"/siteverify", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	return siteverify(req)
}
