package parse

import (
    "io/ioutil"
    "server/web/api/utils"
    "fmt"
    "path/filepath"
    "strings"
    "github.com/anacrolix/torrent"
)

func ParseTorrentSpec(path string) []*torrent.TorrentSpec {
    files, err := ioutil.ReadDir(path)
    if err != nil {
       fmt.Println("err %v", err)
       return nil
    }
    specs := make([]*torrent.TorrentSpec, 0, len(files))
    for _, file := range files {
        filename := filepath.Join(path, file.Name())
        if strings.ToLower(filepath.Ext(file.Name())) == ".torrent" {
            sp, err := utils.ParseLink("file://" + filename)
            if err == nil {
                specs = append(specs, sp)
            }
        }
    }
    return specs
}

func ParseMagnet(mag string) []*torrent.TorrentSpec {
    if !strings.HasPrefix(mag,"magnet") {
       return nil
    }
    sp, err := utils.ParseLink(mag)
    if err != nil {
        fmt.Printf("Parse %s err:%v", mag, err)
        return nil
    }
    r := [1]*torrent.TorrentSpec{sp}
    return r[:]
}