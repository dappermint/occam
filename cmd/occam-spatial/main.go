// Command occam-spatial renders multichannel audio to a binaural stereo pair,
// and upmixes stereo first when there is nothing multichannel to start from.
//
// Deliberately a separate binary from occam. It shares no code with the HID
// tool and nothing here can break a headset that already works.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dappermint/occam/internal/spatial"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "occam-spatial:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		layoutName = flag.String("layout", "", "speaker layout of the input, guessed from channel count when omitted")
		upmixTo    = flag.String("upmix", "", "upmix stereo to this layout first: 5.1, 7.1, 7.1.4")
		out        = flag.String("o", "", "output path, defaults to <input>-binaural.wav")
		modelName  = flag.String("model", "measured", "head model: measured (SADIE II KU100) or synthetic")
		peak       = flag.Float64("peak", 0.95, "normalise the result to this peak, 0 to leave it alone")
		showVer    = flag.Bool("version", false, "print the version")
	)
	flag.Usage = usage
	flag.Parse()

	if *showVer {
		fmt.Println("occam-spatial", version)
		return nil
	}
	if flag.NArg() != 1 {
		usage()
		return fmt.Errorf("need exactly one input file")
	}
	inPath := flag.Arg(0)

	started := time.Now()
	in, err := spatial.ReadWAV(inPath)
	if err != nil {
		return err
	}
	fmt.Printf("  in       %s, %d ch, %d Hz, %.1fs\n",
		filepath.Base(inPath), len(in.Channels), in.Rate,
		float64(in.Frames())/float64(in.Rate))

	// Upmix first when asked, so the renderer always sees a real layout.
	layout, err := resolveLayout(*layoutName, in)
	if err != nil {
		return err
	}
	if *upmixTo != "" {
		target, err := spatial.LayoutByName(*upmixTo)
		if err != nil {
			return err
		}
		if in, err = spatial.Upmix(in, target); err != nil {
			return err
		}
		layout = target
		fmt.Printf("  upmix    stereo to %s, %d ch\n", target.Name, target.Channels())
	}

	fmt.Printf("  layout   %s\n", layout.Name)
	for _, s := range layout.Speakers {
		if s.LFE {
			fmt.Printf("           %-4s low frequency\n", s.Name)
			continue
		}
		fmt.Printf("           %-4s az %+.0f  el %+.0f\n", s.Name, s.Azimuth, s.Elevation)
	}

	model, err := spatial.ModelByName(*modelName)
	if err != nil {
		return err
	}
	renderer, err := spatial.NewRenderer(layout, in.Rate, model)
	if err != nil {
		return err
	}
	fmt.Printf("  model    %s\n", renderer.Model())

	rendered, err := renderer.Render(in)
	if err != nil {
		return err
	}
	fmt.Printf("  peak     %.3f before normalising\n", spatial.Peak(rendered))
	if *peak > 0 {
		spatial.Normalize(rendered, *peak)
	}

	outPath := *out
	if outPath == "" {
		ext := filepath.Ext(inPath)
		outPath = inPath[:len(inPath)-len(ext)] + "-binaural.wav"
	}
	if err := spatial.WriteWAV(outPath, rendered); err != nil {
		return err
	}

	fmt.Printf("  out      %s, 2 ch, 24-bit\n", filepath.Base(outPath))
	fmt.Printf("  took     %s\n", time.Since(started).Round(time.Millisecond))
	return nil
}

func resolveLayout(name string, in spatial.Audio) (spatial.Layout, error) {
	if name != "" {
		return spatial.LayoutByName(name)
	}
	return spatial.LayoutForChannels(len(in.Channels))
}

func usage() {
	fmt.Fprint(os.Stderr, `occam-spatial renders multichannel audio to binaural stereo.

  occam-spatial [flags] <input.wav>

Nothing on macOS emits twelve discrete channels, so the usual path is to
synthesise a layout from stereo and then render that:

  occam-spatial --upmix 7.1.4 track.wav

Already have multichannel audio? Just render it:

  occam-spatial movie-5.1.wav

Flags:
`)
	flag.PrintDefaults()
}
