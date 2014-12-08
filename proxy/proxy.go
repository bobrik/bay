package main

import (
	"flag"
	"github.com/jackpal/Taipei-Torrent/torrent"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
)

func main() {
	tracker := flag.String("tracker", "", "tracker url: http://host:port/")
	listen := flag.String("listen", "0.0.0.0:8888", "bind location")
	root := flag.String("root", "", "root dir to keep working files")
	flag.Parse()

	if *tracker == "" || *root == "" {
		flag.PrintDefaults()
		return
	}

	tu, err := url.Parse(*tracker)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		u := *req.URL

		if req.Host == "localhost" {
			u.Host = "registry-1.docker.io"
		} else {
			u.Host = req.Host
		}

		if req.TLS != nil {
			u.Scheme = "https"
		} else {
			u.Scheme = "http"
		}

		log.Println("got request:", req.Method, u.String())

		if req.Method == "PUT" || !strings.HasPrefix(u.Path, "/v1/images/") || !strings.HasSuffix(u.Path, "/layer") {
			pu := u
			pu.Path = "/"
			proxy := httputil.NewSingleHostReverseProxy(&pu)
			proxy.ServeHTTP(w, req)
			return
		}

		tu := *tu
		q := tu.Query()
		q.Set("url", u.String())

		log.Println(req.URL.String())
		log.Printf("%#v", req.URL)
		log.Println(tu.String() + "?" + q.Encode())

		resp, err := http.Get(tu.String() + "?" + q.Encode())
		if err != nil {
			http.Error(w, "getting torrent failed", http.StatusInternalServerError)
			return
		}

		defer resp.Body.Close()

		f, err := ioutil.TempFile(*root, "image-torrent-")
		if err != nil {
			http.Error(w, "torrent file creation failed", http.StatusInternalServerError)
			return
		}

		defer func() {
			f.Close()
			os.Remove(f.Name())
		}()

		_, err = io.Copy(f, resp.Body)
		if err != nil {
			http.Error(w, "reading torrent contents failed", http.StatusInternalServerError)
			return
		}

		m, err := torrent.GetMetaInfo(nil, f.Name())
		if err != nil {
			http.Error(w, "reading torrent failed", http.StatusInternalServerError)
			return
		}

		err = torrent.RunTorrents(&torrent.TorrentFlags{
			FileDir:   *root,
			SeedRatio: 0,
		}, []string{f.Name()})

		lf := path.Join(*root, m.Info.Name)

		defer os.Remove(lf)

		// TODO: start another RunTorrents for configured interval
		// TODO: and remove data after that
		// TODO: or to hell with it

		if err != nil {
			http.Error(w, "downloading torrent failed", http.StatusInternalServerError)
			return
		}

		l, err := os.Open(lf)
		if err != nil {
			http.Error(w, "layer file open failed", http.StatusInternalServerError)
			return
		}

		defer l.Close()

		io.Copy(w, l)
	})

	err = http.ListenAndServe(*listen, mux)
	if err != nil {
		log.Fatal(err)
	}
}
