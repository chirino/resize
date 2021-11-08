package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/widget"
	"github.com/nfnt/resize"
	"image"
	"image/jpeg"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var maxWidth = 1200
var maxHeight = 0
var maxSize = 250
var files []string
var status binding.String

func main() {
	if len(os.Args) == 1 {
		files = []string{filepath.Dir(os.Args[0])}
	} else {
		files = os.Args[1:]
	}

	a := app.NewWithID("com.github.chirino.resize")
	status = binding.NewString()
	w := a.NewWindow("Welcome to the Image Resize App")
	w.SetContent(newAppContent(a))
	w.Resize(fyne.NewSize(640, 460))
	w.ShowAndRun()
}

func newAppContent(a fyne.App) fyne.CanvasObject {

	maxWidthEntry := widget.NewEntry()
	maxWidthEntry.Text = a.Preferences().StringWithFallback("MaxWidth", fmt.Sprint(maxWidth))
	maxWidthEntry.Validator = validation.NewRegexp(`\d+`, "not a valid width")

	maxHeightEntry := widget.NewEntry()
	maxHeightEntry.Text = a.Preferences().StringWithFallback("MaxHeight", fmt.Sprint(maxHeight))
	maxHeightEntry.Validator = validation.NewRegexp(`\d+`, "not a valid height")

	maxSizeEntry := widget.NewEntry()
	maxSizeEntry.Text = a.Preferences().StringWithFallback("MaxSize", fmt.Sprint(maxSize))
	maxSizeEntry.Validator = validation.NewRegexp(`\d+`, "not a valid size")

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Images:", Widget: widget.NewLabelWithStyle(strings.Join(files, ","), fyne.TextAlignCenter, fyne.TextStyle{})},
			{Text: "Max Width:", Widget: maxWidthEntry, HintText: "max width in pixels"},
			{Text: "Max Height:", Widget: maxHeightEntry, HintText: "max height in pixels"},
			{Text: "Max Size:", Widget: maxSizeEntry, HintText: "max file size in kb"},
		},
		SubmitText: "Resize",
	}

	content := container.NewMax()
	form.OnSubmit = func() {

		maxWidth, _ = strconv.Atoi(maxWidthEntry.Text)
		maxHeight, _ = strconv.Atoi(maxHeightEntry.Text)
		maxSize, _ = strconv.Atoi(maxSizeEntry.Text)

		a.Preferences().SetString("MaxWidth", maxWidthEntry.Text)
		a.Preferences().SetString("MaxHeight", maxHeightEntry.Text)
		a.Preferences().SetString("MaxSize", maxSizeEntry.Text)

		p := widget.NewProgressBarInfinite()
		p.Start()

		content.Objects = []fyne.CanvasObject{container.NewVBox(
			widget.NewLabel("Resizing..."),
			p,
			widget.NewLabelWithData(status))}
		content.Refresh()

		go func() {
			defer func() {
				p.Stop()
				content.Objects = []fyne.CanvasObject{container.NewVBox(
					widget.NewLabelWithData(status),
					widget.NewButton("Ok", func() {
						content.Objects = []fyne.CanvasObject{form}
						content.Refresh()
					}))}
				content.Refresh()
			}()

			count := 0
			for _, file := range files {
				c, err := processFile(file)
				if err != nil {
					status.Set(fmt.Sprintln("error:", err))
					return
				}
				count += c
			}
			status.Set(fmt.Sprintln("resized ", count, "images"))
		}()
	}
	content.Objects = []fyne.CanvasObject{form}

	return container.NewCenter(container.NewVBox(content))

}

func processFile(file string) (int, error) {

	f, err := os.Stat(file)
	if err != nil {
		return 0, err
	}

	if f.IsDir() {

		files, err := ioutil.ReadDir(file)
		if err != nil {
			return 0, err
		}

		count := 0
		for _, child := range files {

			c, err := processFile(filepath.Join(file, child.Name()))
			if err != nil {
				return count, err
			}
			count += c
		}
		return count, nil

	} else {

		name := filepath.Base(file)
		ext := filepath.Ext(name)
		lowerExt := strings.ToLower(ext)
		if lowerExt == ".jpg" || lowerExt == ".jpeg" || lowerExt == ".png" {
			fi, err := os.Open(file)
			if err != nil {
				return 0, err
			}

			img, _, err := image.Decode(fi)
			fi.Close()
			if err != nil {
				return 0, err
			}

			return resizeFile(file, img)
		}
	}
	return 0, nil
}

func resizeFile(file string, img image.Image) (int, error) {
	status.Set(fmt.Sprintln("resizing:", file))
	modified := false

	if maxWidth != 0 && img.Bounds().Max.X > maxWidth {
		modified = true
		img = resize.Resize(uint(maxWidth), 0, img, resize.Lanczos3)
	}
	if maxHeight != 0 && img.Bounds().Max.Y > maxHeight {
		modified = true
		img = resize.Resize(0, uint(maxHeight), img, resize.Lanczos3)
	}

	if !modified {
		stat, err := os.Stat(file)
		if err != nil {
			return 0, err
		}

		// is it already small enough?
		fileSize := int(stat.Size())
		if maxSize == 0 || fileSize < maxSize*1024 {
			return 0, nil
		}
	}

	quality := 100
	if maxSize != 0 {

		// Lets find the best quality setting..
		c := &counter{}
		jpeg.Encode(c, img, &jpeg.Options{Quality: quality})
		if c.count > maxSize*1024 {

			// Then use a binary search to find a a quality setting that makes it small enough
			quality = binarySearch(maxSize*1024, 0, 100, func(quality int) int {
				c := &counter{}
				jpeg.Encode(c, img, &jpeg.Options{Quality: quality})
				return c.count
			})
		}

	}

	return 1, backupOrRestore(file, func() error {
		// change the extension to .jpg
		newName := strings.TrimSuffix(file, filepath.Ext(file)) + ".jpg"

		out, err := os.Create(newName)
		if err != nil {
			return err
		}
		defer out.Close()

		return jpeg.Encode(out, img, &jpeg.Options{Quality: quality})
	})
}

func backupOrRestore(file string, action func() error) error {
	os.Rename(file, file+".backup")
	err := action()
	if err != nil {
		os.Rename(file+".backup", file)
		return err
	}
	os.Remove(file + ".backup")
	return nil
}

func binarySearch(target int, low int, high int, f func(index int) int) int {
	if high < low {
		return -1
	}
	mid := (low + high) / 2
	midValue := f(mid)
	if midValue > target {
		if high == mid {
			return mid
		}
		return binarySearch(target, low, mid, f)
	} else if midValue < target {
		if low == mid {
			return mid
		}
		return binarySearch(target, mid, high, f)
	} else {
		return mid
	}
}

type counter struct {
	count int
}

func (c *counter) Write(p []byte) (int, error) {
	n := len(p)
	c.count += n
	return n, nil
}

var _ io.Writer = &counter{}
