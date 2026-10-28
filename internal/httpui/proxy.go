package httpui

import (
	"fmt"
	"net"
	"strings"

	"github.com/labstack/echo/v5"
)

func (a *App) ConfigureProxies(value string) error {
	if strings.TrimSpace(value) == "" {
		a.Echo.IPExtractor = echo.ExtractIPDirect()
		return nil
	}
	options := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustPrivateNet(false), echo.TrustLinkLocal(false)}
	for _, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		_, block, err := net.ParseCIDR(raw)
		if err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q: %w", raw, err)
		}
		options = append(options, echo.TrustIPRange(block))
	}
	a.Echo.IPExtractor = echo.ExtractIPFromXFFHeader(options...)
	return nil
}
