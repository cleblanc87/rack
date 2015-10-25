package controllers

import (
	"net/http"
	"sort"
	"strings"

	"github.com/convox/rack/Godeps/_workspace/src/github.com/gorilla/mux"
	"github.com/convox/rack/Godeps/_workspace/src/golang.org/x/net/websocket"
	"github.com/convox/rack/api/httperr"
	"github.com/convox/rack/api/models"
)

func AppList(rw http.ResponseWriter, r *http.Request) *httperr.Error {
	apps, err := models.ListApps()

	if err != nil {
		return httperr.Server(err)
	}

	sort.Sort(apps)

	return RenderJson(rw, apps)
}

func AppShow(rw http.ResponseWriter, r *http.Request) *httperr.Error {
	app := mux.Vars(r)["app"]

	a, err := models.GetApp(mux.Vars(r)["app"])

	if awsError(err) == "ValidationError" {
		return httperr.Errorf(404, "no such app: %s", app)
	}

	if err != nil && strings.HasPrefix(err.Error(), "no such app") {
		return httperr.Errorf(404, "no such app: %s", app)
	}

	if err != nil {
		return httperr.Server(err)
	}

	return RenderJson(rw, a)
}

func AppCreate(rw http.ResponseWriter, r *http.Request) *httperr.Error {
	name := r.FormValue("name")

	app := &models.App{
		Name: name,
	}

	err := app.Create()

	if awsError(err) == "AlreadyExistsException" {
		app, err := models.GetApp(name)

		if err != nil {
			return httperr.Server(err)
		}

		return httperr.Errorf(403, "there is already an app named %s (%s)", name, app.Status)
	}

	if err != nil {
		return httperr.Server(err)
	}

	app, err = models.GetApp(name)

	if err != nil {
		return httperr.Server(err)
	}

	return RenderJson(rw, app)
}

func AppDelete(rw http.ResponseWriter, r *http.Request) *httperr.Error {
	name := mux.Vars(r)["app"]

	app, err := models.GetApp(name)

	if awsError(err) == "ValidationError" {
		return httperr.Errorf(404, "no such app: %s", name)
	}

	if err != nil {
		return httperr.Server(err)
	}

	err = app.Delete()

	if err != nil {
		return httperr.Server(err)
	}

	return RenderSuccess(rw)
}

func AppLogs(ws *websocket.Conn) *httperr.Error {
	app := mux.Vars(ws.Request())["app"]

	a, err := models.GetApp(app)

	if awsError(err) == "ValidationError" {
		return httperr.Errorf(404, "no such app: %s", app)
	}

	if err != nil {
		return httperr.Server(err)
	}

	logs := make(chan []byte)
	done := make(chan bool)

	a.SubscribeLogs(logs, done)

	for data := range logs {
		ws.Write(data)
	}

	return nil
}
