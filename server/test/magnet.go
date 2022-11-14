package main

import (
    "fmt"
    "os"
    "server/test/parse"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Printf("Please input magnet")
        return
    }
    mag := os.Args[1]
    specs := parse.ParseMagnet(mag)
    for _,sp := range specs {
        fmt.Printf("--------------- \n")
        fmt.Printf("Title: %s,Hash:%s   \n", sp.DisplayName, sp.InfoHash.HexString())
        if sp.Trackers == nil || len(sp.Trackers) == 0 {
            fmt.Printf("Mag:%s,no trackers:\n ",sp.DisplayName)
            continue
        }
        fmt.Printf("Trackers:\n ")
        for _,t := range sp.Trackers[0] {
            fmt.Println(t)
        }
    }
    fmt.Printf("Parse magnet:%s files finished\n", mag)
}


