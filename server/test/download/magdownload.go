package main

import (
    "server/torr"
    "fmt"
    "os"
    "server/test/parse"
    "server/log"
    "server/settings"
    "time"
)

func main() {
    if len(os.Args) < 2 {
       fmt.Printf("Please input magnet")
       return
    }
    mag := os.Args[1]
    specs := parse.ParseMagnet(mag)
    if specs == nil {
        fmt.Printf("No magnet spec")
        return
    }
    settings.InitSets(false)
    BTS := torr.NewBTS()
    err := BTS.Connect()
    if err != nil {
       fmt.Printf("Bts server connect err:%v", err)
    }
	spec := specs[0]
    torrent, err := torr.NewTorrent(spec, BTS)
    if err != nil {
        log.TLogln("error add torrent:", err)
        return
    }
    torrent.WaitInfo()
    // TODO download
    //torrent.Watch()
    state := torrent.Torrent.Stats()
    fmt.Printf("torrent:%s, total peer:%d/%d/%d \n", spec.DisplayName,
    state.TotalPeers, state.PendingPeers, state.ActivePeers)
    torrent.Torrent.DownloadAll()
    watch(torrent)
}

func watch(torr *torr.Torrent){
    for {
        state := torr.Torrent.Stats()
        fmt.Println("---------------------------------------------")
        fmt.Printf("Title:%s,peers:%d/%d/%d,seeders:%d/%d,chunk:%d/%d,speed:%g,total size:%d,pieces:%d,read bytes:%d  \n",
        torr.TorrentSpec.DisplayName,
        state.TotalPeers, state.PendingPeers, state.ActivePeers,
        state.ConnectedSeeders, state.HalfOpenPeers,
        state.ChunksRead.Int64(), state.ChunksReadUseful.Int64(),
        torr.DownloadSpeed, torr.Torrent.Length(),torr.Torrent.NumPieces(), torr.BytesReadUsefulData)
        for _, file := range torr.Torrent.Files() {
            fmt.Printf("file offset:%d,%s,%s,%d \n", file.Offset(),file.Path(),file.DisplayPath(),
            file.Length())
        }
        time.Sleep(10*time.Second)
    }
}

// func watch(torrent torr.Torrent) {
//     progressTicker := time.NewTicker(time.Second)
// 	defer progressTicker.Stop()
// 	for {
// 		select {
// 		case <-progressTicker.C:
//
// 		case <-t.closed:
// 			return
// 		}
// 	}
// }