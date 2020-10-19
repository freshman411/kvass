package shard

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/prometheus/config"
	"github.com/sirupsen/logrus"
	"tkestack.io/kvass/pkg/api"
	"tkestack.io/kvass/pkg/prom"
	"tkestack.io/kvass/pkg/utils/types"
)

// Web is the api server of coordinator
type Web struct {
	*gin.Engine
	ConfigReload chan *config.Config
	lg           logrus.FieldLogger
	readConfig   func() ([]byte, error)
	promUrl      string
	paths        []string
	runtime      *RuntimeManager
}

func NewWeb(
	promUrl string,
	readConfig func() ([]byte, error),
	runtime *RuntimeManager,
	lg logrus.FieldLogger) *Web {
	w := &Web{
		ConfigReload: make(chan *config.Config, 2),
		Engine:       gin.Default(),
		lg:           lg,
		readConfig:   readConfig,
		promUrl:      promUrl,
		runtime:      runtime,
	}

	w.POST(w.handlePath("/api/v1/shard/runtimeinfo"), api.Wrap(w.lg, w.updateRuntimeInfo))
	w.GET(w.handlePath("/api/v1/shard/runtimeinfo"), api.Wrap(w.lg, w.runtimeInfo))
	w.GET(w.handlePath("/api/v1/targets"), api.Wrap(w.lg, w.targets))
	w.GET(w.handlePath("/api/v1/status/config"), api.Wrap(lg, func(ctx *gin.Context) *api.Result {
		return prom.ApiReadConfig(ctx, readConfig)
	}))
	w.POST(w.handlePath("/-/reload"), api.Wrap(lg, func(ctx *gin.Context) *api.Result {
		return prom.ApiReloadConfig(readConfig, w.ConfigReload)
	}))
	return w
}

func (w *Web) handlePath(path string) string {
	w.paths = append(w.paths, path)
	return path
}

func (w *Web) Run(address string) error {
	return http.ListenAndServe(address, w)
}

func (w *Web) ServeHTTP(wr http.ResponseWriter, r *http.Request) {
	if types.FindString(r.URL.Path, w.paths...) {
		w.Engine.ServeHTTP(wr, r)
		return
	}

	u, _ := url.Parse(w.promUrl)
	r.URL.Host = u.Host
	r.URL.Scheme = u.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = u.Host
	httputil.NewSingleHostReverseProxy(u).ServeHTTP(wr, r)
}

func (w *Web) runtimeInfo(g *gin.Context) *api.Result {
	ret, err := w.runtime.RuntimeInfo()
	if err != nil {
		return api.InternalErr(err, "")
	}
	return api.Data(ret)
}

func (w *Web) updateRuntimeInfo(g *gin.Context) *api.Result {
	data := &RuntimeInfo{}
	if err := g.BindJSON(&data); err != nil {
		return api.InternalErr(err, "get data failed")
	}

	if err := w.runtime.Update(data); err != nil {
		return api.InternalErr(err, "update runtime")
	}

	return api.Data(nil)
}

func (w *Web) targets(g *gin.Context) *api.Result {
	ts, err := w.runtime.Targets(g.Query("state"))
	if err != nil {
		return api.InternalErr(err, "")
	}
	return api.Data(ts)
}
