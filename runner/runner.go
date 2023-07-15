package runner

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lollipopkit/gommon/log"
	"github.com/lollipopkit/server_box_monitor/model"
	"github.com/lollipopkit/server_box_monitor/res"
	"github.com/lollipopkit/server_box_monitor/web"
)

var (
	pushPairs     = []*model.PushPair{}
	pushPairsLock = new(sync.RWMutex)
)

func init() {
	scriptBytes, err := res.Files.ReadFile(res.ServerBoxShellFileName)
	if err != nil {
		log.Err("[INIT] Read embed file error: %v", err)
		panic(err)
	}
	err = os.WriteFile(res.ServerBoxShellPath, scriptBytes, 0755)
	if err != nil {
		log.Err("[INIT] Write script file error: %v", err)
		panic(err)
	}
}

func Start() {
	go runWeb()
	go runCheck()
	// 阻塞主线程
	select {}
}

func runCheck() {
	err := model.ReadAppConfig()
	if err != nil {
		log.Err("[CONFIG] Read app config error: %v", err)
		panic(err)
	}

	for range time.NewTicker(model.GetInterval()).C {
		err = model.RefreshStatus()
		status := model.Status
		if err != nil {
			log.Warn("[STATUS] Get status error: %v", err)
			continue
		}

		for _, rule := range model.Config.Rules {
			notify, pushPair, err := rule.ShouldNotify(status)
			if err != nil {
				if !strings.Contains(err.Error(), model.ErrNotReady.Error()) {
					log.Warn("[RULE] %s error: %v", rule.Id(), err)
				}
			}

			if notify && pushPair != nil {
				pushPairsLock.Lock()
				pushPairs = append(pushPairs, pushPair)
				pushPairsLock.Unlock()
			}
		}

		if len(pushPairs) == 0 {
			continue
		}

		log.Info("[PUSH] %d to push", len(pushPairs))

		pushPairsLock.RLock()
		for _, push := range model.Config.Pushes {
			err := push.Push(pushPairs)
			if err != nil {
				log.Warn("[PUSH] %s error: %v", push.Name, err)
				continue
			}
			log.Suc("[PUSH] %s success", push.Name)
		}
		pushPairsLock.RUnlock()

		pushPairsLock.Lock()
		pushPairs = []*model.PushPair{}
		pushPairsLock.Unlock()
	}
}

func runWeb() {
	e := echo.New()

	e.Use(middleware.Recover())
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(3)))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{},
	}))
	e.HideBanner = true

	e.GET("/status", web.Status)

	e.Logger.Fatal(e.Start(":3770"))
}
