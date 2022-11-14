package main

import (
    "fmt"
    "os"
    "path/filepath"
    "server/test/parse"
)

func main() {
    dir := os.Args[1]
    path, err := filepath.Abs(os.Args[1])
    if err != nil {
        path = dir
    }
    specs := parse.ParseTorrentSpec(path)
    for _,sp := range specs {
        fmt.Printf("--------------- \n")
        fmt.Printf("Title: %s,Hash:%s   \n", sp.DisplayName, sp.InfoHash.HexString())
        fmt.Printf("Trackers:\n ")
        for _,t := range sp.Trackers[0] {
            fmt.Println(t)
        }
    }
    fmt.Printf("Parse dir:%s torrent files finished\n", dir)
}


