package main

import "log"
import "flag"
import "github.com/jackpal/Taipei-Torrent/tracker"
import "github.com/jackpal/Taipei-Torrent/torrent"
import "os"
import "net/http"
import "math"
import "sync"

type Tracker struct {
	mutex    sync.Mutex
	requests map[string]*sync.Mutex

	downloader *Downloader

	flags        *torrent.TorrentFlags
	conns        chan *torrent.BtConn
	sessions     map[string]*torrent.TorrentSession
	sessionCh    chan *torrent.TorrentSession
	sessionMutex sync.RWMutex

	tr       *tracker.Tracker
	trListen string

	mux    *http.ServeMux
	listen string

	port int
}

func NewTracker(listen, trListen, root string, port int) (*Tracker, error) {
	flags := &torrent.TorrentFlags{
		Port:                port,
		FileDir:             root,
		SeedRatio:           math.Inf(0),
		UseDeadlockDetector: true,
	}

	conns, listenPort, err := torrent.ListenForPeerConnections(flags)

	if err != nil {
		return nil, err
	}

	tr := tracker.NewTracker()
	tr.Addr = trListen

	mux := http.NewServeMux()

	t := &Tracker{
		mutex:    sync.Mutex{},
		requests: map[string]*sync.Mutex{},

		downloader: NewDownloader(root),

		flags:        flags,
		conns:        conns,
		sessions:     map[string]*torrent.TorrentSession{},
		sessionCh:    make(chan *torrent.TorrentSession),
		sessionMutex: sync.RWMutex{},

		tr:       tr,
		trListen: trListen,

		mux:    mux,
		listen: listen,

		port: listenPort,
	}

	mux.HandleFunc("/", t.handle)

	return t, nil
}

func (t *Tracker) ensureTorrentExists(file string, trkr string) (string, error) {
	tf := file + ".torrent"

	if _, err := os.Stat(tf); os.IsNotExist(err) {
		m, err := torrent.CreateMetaInfoFromFileSystem(nil, file, trkr, 0, true)
		if err != nil {
			return tf, err
		}

		m.Announce = "http://" + t.trListen + "/announce"

		meta, err := os.Create(tf)
		if err != nil {
			return tf, err
		}

		defer meta.Close()

		err = m.Bencode(meta)
		if err != nil {
			return tf, err
		}
	}

	return tf, nil
}

func (t *Tracker) handleSafely(w http.ResponseWriter, u string) error {
	f, err := t.downloader.Download(u)
	if err != nil {
		return err
	}

	tf, err := t.ensureTorrentExists(f, t.trListen)
	if err != nil {
		return err
	}

	m, err := torrent.GetMetaInfo(nil, tf)
	if err != nil {
		return err
	}

	m.Bencode(w)

	ts, err := torrent.NewTorrentSession(t.flags, tf, uint16(t.port))
	if err != nil {
		return err
	}

	t.sessionCh <- ts

	return nil
}

func (t *Tracker) handle(w http.ResponseWriter, req *http.Request) {
	u := req.URL.Query().Get("url")
	if u == "" {
		http.Error(w, "no url provided", http.StatusBadRequest)
		return
	}

	log.Println("dealing with " + u)

	t.mutex.Lock()

	// existing mutex: just wait for it to unlock
	if e, ok := t.requests[u]; ok {
		log.Println("waiting for current download to finish")
		t.mutex.Unlock()
		e.Lock()

		// here file is downloaded already or failed
		// and "e" is no longer referenced from the map
		t.handle(w, req)
		return
	}

	// new mutex: create it and put into map
	m := &sync.Mutex{}
	t.requests[u] = m

	m.Lock()
	defer m.Unlock()

	t.mutex.Unlock()

	// actually do some job
	err := t.handleSafely(w, u)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// delete mutex from the map
	t.mutex.Lock()
	delete(t.requests, u)
	t.mutex.Unlock()
}

func (t *Tracker) Start() error {
	errch := make(chan error)

	go func() {
		for ts := range t.sessionCh {
			t.sessionMutex.Lock()

			if _, ok := t.sessions[ts.M.InfoHash]; ok {
				t.sessionMutex.Unlock()
				continue
			}

			t.sessions[ts.M.InfoHash] = ts

			t.sessionMutex.Unlock()

			err := t.tr.Register(ts.M.InfoHash, ts.M.Info.Name)
			if err != nil {
				errch <- err
			}

			go ts.DoTorrent()

			log.Println("seeding " + ts.M.Info.Name)
		}
	}()

	go func() {
		for c := range t.conns {
			t.sessionMutex.RLock()

			if ts, ok := t.sessions[c.Infohash]; ok {
				log.Printf("New bt connection for ih %x", c.Infohash)
				ts.AcceptNewPeer(c)
			} else {
				log.Printf("Unknown hash: %x", c.Infohash)
			}

			t.sessionMutex.RUnlock()
		}
	}()

	go func() {
		err := http.ListenAndServe(t.listen, t.mux)
		if err != nil {
			errch <- err
		}
	}()

	go func() {
		err := t.tr.ListenAndServe()
		if err != nil {
			errch <- err
		}
	}()

	return <-errch
}

func main() {
	listen := flag.String("listen", "", "bind location")
	addr := flag.String("tracker", "0.0.0.0:6881", "tracker location")
	root := flag.String("root", "", "root dir to keep working files")
	port := flag.Int("port", 7788, "peering port")
	flag.Parse()

	if *listen == "" || *root == "" {
		flag.PrintDefaults()
		return
	}

	tr, err := NewTracker(*listen, *addr, *root, *port)
	if err != nil {
		log.Fatal(err)
	}

	err = tr.Start()
	if err != nil {
		log.Fatal(err)
	}
}
