package downloadFile

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
)

func DownloadFile(url string) (string, error) {
	resp, err := http.Get(url)
	filename := path.Base(resp.Request.URL.Path)
	out, _ := os.Create(filename)
	defer out.Close()
	if err != nil {
		fmt.Println(err)
	}
	if resp != nil {
		defer resp.Body.Close()
		io.Copy(out, resp.Body)
		return filename, nil
	}
	return "no file name", err
}
