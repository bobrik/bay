package main

import "log"
import "flag"
import "github.com/jackpal/Taipei-Torrent/tracker"
import "github.com/jackpal/Taipei-Torrent/torrent"
import "os"
import "net/http"
import "github.com/bobrik/bay"
import "math"

func main() {
	listen := flag.String("listen", "0.0.0.0:8080", "bind location")
	addr := flag.String("tracker", "0.0.0.0:6881", "tracker location")
	root := flag.String("root", "", "root dir to keep working files")
	port := flag.Int("port", 7788, "peering port")
	flag.Parse()

	if *listen == "" || *root == "" {
		flag.PrintDefaults()
		return
	}

	tr := tracker.NewTracker()
	tr.Addr = *addr

	flags := &torrent.TorrentFlags{
		Port:                *port,
		FileDir:             *root,
		SeedRatio:           math.Inf(0),
		UseDeadlockDetector: true,
	}

	conChan, listenPort, err := torrent.ListenForPeerConnections(flags)

	torrentSessions := make(map[string]*torrent.TorrentSession)

	go func() {
		for c := range conChan {
			log.Printf("New bt connection for ih %x", c.Infohash)
			if ts, ok := torrentSessions[c.Infohash]; ok {
				ts.AcceptNewPeer(c)
			}
		}
	}()

	d := bay.NewDownloader(*root)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		u := req.URL.Query().Get("url")
		if u == "" {
			http.Error(w, "no url provided", http.StatusBadRequest)
			return
		}

		f, err := d.Download(u)
		if err != nil {
			http.Error(w, "download failed", http.StatusInternalServerError)
			return
		}

		t := f + ".torrent"

		if _, err := os.Stat(t); err != nil {
			m, err := torrent.CreateMetaInfoFromFileSystem(nil, f, 0, true)
			if err != nil {
				http.Error(w, "torrent creation failed", http.StatusInternalServerError)
				return
			}

			m.Announce = "http://" + *addr + "/announce"

			tf, err := os.Create(t)
			if err != nil {
				http.Error(w, "torrent saving failed", http.StatusInternalServerError)
				return
			}

			defer tf.Close()

			err = m.Bencode(tf)
			if err != nil {
				http.Error(w, "bencode failed", http.StatusInternalServerError)
				return
			}
		}

		m, err := torrent.GetMetaInfo(nil, t)
		if err != nil {
			http.Error(w, "torrent reading failed", http.StatusInternalServerError)
			return
		}

		m.Bencode(w)

		tr.Register(m.InfoHash, m.Info.Name)

		ts, err := torrent.NewTorrentSession(flags, t, uint16(listenPort))
		if err != nil {
			http.Error(w, "torrent session creation failed", http.StatusInternalServerError)
			return
		}

		// TODO: mutex
		if _, seeding := torrentSessions[ts.M.InfoHash]; seeding {
			return
		}

		torrentSessions[ts.M.InfoHash] = ts

		go ts.DoTorrent()

		log.Println("seeding " + u)
	})

	go func() {
		http.ListenAndServe(*listen, mux)
	}()

	err = tr.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
