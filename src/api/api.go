package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/polyfloyd/trollibox/src/filter"
	"github.com/polyfloyd/trollibox/src/library"
	"github.com/polyfloyd/trollibox/src/library/netmedia"
	"github.com/polyfloyd/trollibox/src/library/raw"
	"github.com/polyfloyd/trollibox/src/library/stream"
	"github.com/polyfloyd/trollibox/src/player"
	"github.com/polyfloyd/trollibox/src/util"
	"golang.org/x/net/websocket"
)

// InitRouter attaches all API routes to the specified router.
func InitRouter(r chi.Router, players player.List, netServer *netmedia.Server, filterdb *filter.DB, streamdb *stream.DB, rawServer *raw.Server) {
	r.Route("/player/{playerName}", func(r chi.Router) {
		api := playerAPI{
			players:   players,
			libs:      []library.Library{streamdb, rawServer},
			netServer: netServer,
			rawServer: rawServer,
		}
		r.Use(jsonCtx)
		r.Use(api.playerCtx)
		r.Route("/playlist", func(r chi.Router) {
			r.Get("/", api.playlistContents)
			r.Put("/", api.playlistInsert)
			r.Patch("/", api.playlistMove)
			r.Delete("/", api.playlistRemove)
			r.Post("/appendraw", api.rawTrackAdd)
			r.Post("/appendnet", api.netTrackAdd)
		})
		r.Post("/current", api.setCurrent)
		r.Post("/next", api.next) // Deprecated
		r.Get("/time", api.getTime)
		r.Post("/time", api.setTime)
		r.Get("/playstate", api.getPlaystate)
		r.Post("/playstate", api.setPlaystate)
		r.Get("/volume", api.getVolume)
		r.Post("/volume", api.setVolume)
		r.Get("/list/", api.listStoredPlaylists)
		r.Get("/list/{name}/", api.storedPlaylistTracks)
		r.Get("/tracks", api.tracks)
		r.Get("/tracks/search", api.trackSearch)
		r.Get("/tracks/art", api.trackArt)
		r.Mount("/listen", http.HandlerFunc(api.listen))
	})

	r.Route("/filters/", func(r chi.Router) {
		api := filterAPI{db: filterdb}
		r.Get("/", api.list)
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", api.get)
			r.Delete("/", api.remove)
			r.Put("/", api.set)
		})
		r.Mount("/listen", websocket.Handler(htListen(&filterdb.Emitter)))
	})

	r.Route("/streams", func(r chi.Router) {
		api := streamsAPI{db: streamdb}
		r.Get("/", api.list)
		r.Post("/", api.add)
		r.Delete("/", api.remove)
		r.Mount("/listen", websocket.Handler(htListen(&streamdb.Emitter)))
	})

	r.Mount("/raw", rawServer)
}

// WriteError writes an error to the client or an empty object if err is nil.
//
// An attempt is made to tune the response format to the requestor.
func WriteError(req *http.Request, res http.ResponseWriter, err error) {
	log.Printf("Error serving %s: %v", req.RemoteAddr, err)
	res.WriteHeader(http.StatusBadRequest)

	if req.Header.Get("X-Requested-With") == "" {
		res.Write([]byte(err.Error()))
		return
	}

	data, _ := json.Marshal(err)
	if data == nil {
		data = []byte("{}")
	}
	json.NewEncoder(res).Encode(map[string]interface{}{
		"error": err.Error(),
		"data":  (*json.RawMessage)(&data),
	})
}

func htListen(emitter *util.Emitter) func(*websocket.Conn) {
	return func(conn *websocket.Conn) {
		ch := emitter.Listen()
		defer emitter.Unlisten(ch)
		conn.SetDeadline(time.Time{})
		// TODO: All these events should not all be combined in here.
		for event := range ch {
			var eventStr string
			switch t := event.(type) {
			case player.Event:
				eventStr = string(t)
			case library.UpdateEvent:
				eventStr = "tracks"
			case filter.UpdateEvent:
				eventStr = "update"
			default:
				continue
			}
			if _, err := conn.Write([]uint8(eventStr)); err != nil {
				break
			}
		}
	}
}

func jsonCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(res, req)
	})
}
