package views

import (
	"context"
	"strings"

	"github.com/lieranderl/aestheticlab/internal/i18n"
)

// tr is msg in the locale of the page being rendered.
func tr(ctx context.Context, msg string) string { return i18n.T(ctx, msg) }

// trf formats args with the translation of format ("Book with %v").
func trf(ctx context.Context, format string, args ...any) string {
	return i18n.Tf(ctx, format, args...)
}

// trc is msg where context tells it apart from the same English elsewhere.
func trc(ctx context.Context, context, msg string) string { return i18n.C(ctx, context, msg) }

// pageLang is the page's locale, for <html lang>.
func pageLang(ctx context.Context) string { return string(i18n.FromContext(ctx)) }

// jsQuote escapes text for a single-quoted JavaScript string.
var jsQuote = strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`, " ", ` `, " ", ` `)

// jsString quotes s for a Datastar expression, in single quotes like the
// rest of the page's expressions.
func jsString(s string) string { return "'" + jsQuote.Replace(s) + "'" }

// trJS is msg translated and quoted for a Datastar expression.
func trJS(ctx context.Context, msg string) string { return jsString(tr(ctx, msg)) }
