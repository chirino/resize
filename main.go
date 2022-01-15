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
	"github.com/sqweek/dialog"
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
var convertPngs = true
var files []string
var status binding.String

func main() {

	a := app.NewWithID("com.github.chirino.resize")
	status = binding.NewString()
	w := a.NewWindow("Welcome to the Image Resize App")
	w.SetContent(newAppContent(a))
	w.Resize(fyne.NewSize(640, 460))
	w.ShowAndRun()
}

func newAppContent(a fyne.App) fyne.CanvasObject {

	if len(os.Args) == 1 {
		files = []string{a.Preferences().StringWithFallback("Images", "")}
	} else {
		files = os.Args[1:]
	}

	filesEntry := widget.NewLabel(strings.Join(files, "\n"))
	filesContainer := container.NewVBox(
		container.NewMax(filesEntry),
		widget.NewButton("...", func() {
			dir, err := dialog.Directory().Title("Load images").Browse()
			if err == nil {
				a.Preferences().SetString("Images", dir)
				files = []string{dir}
				filesEntry.Text = strings.Join(files, "\n")
				filesEntry.Refresh()
			}
		}),
	)

	maxWidthEntry := widget.NewEntry()
	maxWidthEntry.Text = a.Preferences().StringWithFallback("MaxWidth", fmt.Sprint(maxWidth))
	maxWidthEntry.Validator = validation.NewRegexp(`\d+`, "not a valid width")
	maxWidthEntry.OnChanged = func(s string) {
		var err error
		maxWidth, err = strconv.Atoi(s)
		if err != nil {
			a.Preferences().SetString("MaxWidth", s)
		}
	}

	maxHeightEntry := widget.NewEntry()
	maxHeightEntry.Text = a.Preferences().StringWithFallback("MaxHeight", fmt.Sprint(maxHeight))
	maxHeightEntry.Validator = validation.NewRegexp(`\d+`, "not a valid height")
	maxHeightEntry.OnChanged = func(s string) {
		var err error
		maxHeight, err = strconv.Atoi(s)
		if err != nil {
			a.Preferences().SetString("MaxHeight", s)
		}
	}

	maxSizeEntry := widget.NewEntry()
	maxSizeEntry.Text = a.Preferences().StringWithFallback("MaxSize", fmt.Sprint(maxSize))
	maxSizeEntry.Validator = validation.NewRegexp(`\d+`, "not a valid size")
	maxHeightEntry.OnChanged = func(s string) {
		var err error
		maxSize, err = strconv.Atoi(s)
		if err != nil {
			a.Preferences().SetString("MaxSize", s)
		}
	}

	convertPngsEntry := widget.NewCheck("Convert .png images", func(b bool) {
		convertPngs = b
		a.Preferences().SetBool("ConvertPngs", b)
	})
	convertPngsEntry.SetChecked(a.Preferences().BoolWithFallback("ConvertPngs", convertPngs))

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Images:", Widget: filesContainer},
			{Text: "Max Width:", Widget: maxWidthEntry, HintText: "max width in pixels"},
			{Text: "Max Height:", Widget: maxHeightEntry, HintText: "max height in pixels"},
			{Text: "Max Size:", Widget: maxSizeEntry, HintText: "max file size in kb"},
			{Text: "", Widget: convertPngsEntry},
		},
		SubmitText: "Resize",
	}

	content := container.NewMax()
	form.OnSubmit = func() {
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
		if lowerExt == ".jpg" || lowerExt == ".jpeg" || (convertPngs && lowerExt == ".png") {

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

		newName := file

		if strings.ToLower(filepath.Ext(file)) == ".png" {
			// change the extension to .jpg
			newName = strings.TrimSuffix(file, filepath.Ext(file)) + ".jpg"
		}

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
