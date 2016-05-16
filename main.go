package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nfnt/resize"
)

var (
	maxRating = flag.Float64("max", 1.1, "maximum deviation detected")
	width     = flag.Int("width", 0, "width to resize to.  omitting either width or height will maintain proportion")
	height    = flag.Int("height", 0, "height to resize to.  omitting either width or height will maintain proportion")
	infile    = flag.String("if", "", "file to process")
	outfile   = flag.String("of", "", "output file")
	cores     = flag.Int("cores", runtime.NumCPU(), "how many cores to use")
)

func init() {

	flag.Parse()

	if *infile == "" || *outfile == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)

}

func main() {

	start := time.Now()

	file, err := os.Open(*infile)
	if err != nil {
		log.Fatal(err)
	}

	info, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}

	img, _, err := image.Decode(file)
	if err != nil {
		log.Fatal(err)
	}

	if err := file.Close(); err != nil {
		log.Fatal(err)
	}

	if *width > 0 || *height > 0 {
		img = resize.Resize(uint(*width), uint(*height), img, resize.Lanczos3)
	}

	const maxQuality = 100

	bestjpeg := jpegToPNG(img, maxQuality)

	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				fmt.Print(".")
			}
		}
	}()

	quality := karySearch(maxQuality, *cores, func(q int) bool {

		current := jpegToPNG(img, q)

		var buf bytes.Buffer
		cmd := exec.Command("compare_pngs", bestjpeg, current)
		cmd.Env = []string{}
		cmd.Stderr = os.Stderr
		cmd.Stdout = &buf
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}

		rating, err := strconv.ParseFloat(strings.TrimSpace(buf.String()), 64)
		if err != nil {
			log.Fatal(err)
		}

		if err := os.Remove(current); err != nil {
			log.Fatal(err)
		}

		return rating < *maxRating

	})

	toJPEG(*outfile, img, quality)

	outinfo, err := os.Stat(*outfile)
	if err != nil {
		log.Fatal(err)
	}

	close(done)

	fmt.Println("\nCompleted in", time.Since(start))
	fmt.Println("Best JPG quality:", quality)
	fmt.Println(*infile+":", human(info.Size()))
	fmt.Println(*outfile+":", human(outinfo.Size()))

}

var sizes []string = []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}

func human(b int64) string {
	var i int
	n := float64(b)
	for n >= 1024 {
		i++
		n /= 1024
	}
	return strconv.FormatFloat(n, 'f', 1, 64) + sizes[i]

}

func toJPEG(name string, img image.Image, quality int) {

	file, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}

	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: quality}); err != nil {
		log.Fatal(err)
	}

	if err := file.Close(); err != nil {
		log.Fatal(err)
	}

}

func jpegToPNG(img image.Image, quality int) string {

	var buf bytes.Buffer

	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		log.Fatal(err)
	}

	tmpimg, err := jpeg.Decode(&buf)
	if err != nil {
		log.Fatal(err)
	}

	pngout, err := ioutil.TempFile(os.TempDir(), "_butter_"+strconv.Itoa(quality)+".png")
	if err != nil {
		log.Fatal(err)
	}

	if err := png.Encode(pngout, tmpimg); err != nil {
		log.Fatal(err)
	}

	if err := pngout.Close(); err != nil {
		log.Fatal(err)
	}

	return pngout.Name()

}

func karySearch(n, k int, f func(int) bool) int {

	if k < 2 {
		k = 2
	}

	var search func(start, end int) int

	search = func(start, end int) int {

		type resp struct {
			i  int
			ok bool
		}

		var size, chunk int

		if end-start > k {
			chunk = (end - start) / k
			size = k
		} else {
			chunk = 1
			size = end - start
		}

		resps := make(chan resp, size)

		for i := k; i > 0; i-- {
			go func(i int) {
				resps <- resp{i: i, ok: f(i)}
			}(start + (i * chunk))
		}

		for i := 0; i < cap(resps); i++ {
			r := <-resps
			// start should always be !ok
			// end should always be ok
			if !r.ok && r.i > start && r.i < end {
				start = r.i
			} else if r.ok && r.i < end && r.i > start {
				end = r.i
			}
		}

		if end-start == 1 {
			return end
		}

		return search(start, end)

	}

	return search(-1, n)

}
