// Command occam-spatial renders multichannel audio to a binaural stereo pair,
// and upmixes stereo first when there is nothing multichannel to start from.
//
// Deliberately a separate binary from occam. It shares no code with the HID
// tool and nothing here can break a headset that already works.
package main

import (
	"flag"
	"fmt"
	"io"
	"math"
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
		gainDB     = flag.Float64("gain", 0, "gain in dB applied to the render, for level matching")
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

	model, err := spatial.ModelByName(*modelName)
	if err != nil {
		return err
	}

	started := time.Now()
	src, err := spatial.OpenWAV(inPath)
	if err != nil {
		return err
	}
	defer src.Close()

	fmt.Printf("  in       %s, %d ch, %d Hz, %.1fs\n",
		filepath.Base(inPath), src.Channels, src.Rate,
		float64(src.Frames())/float64(src.Rate))

	target, err := resolveTarget(*layoutName, *upmixTo, src)
	if err != nil {
		return err
	}
	if src.Channels != 2 && target.Channels() != src.Channels {
		return fmt.Errorf("occam-spatial: input has %d channels, cannot render it as %s",
			src.Channels, target.Name)
	}

	pipe, err := spatial.NewPipeline(target, src.Rate, blockFrames, model)
	if err != nil {
		return err
	}
	fmt.Printf("  layout   %s\n  model    %s\n", target.Name, pipe.Model())

	outPath := *out
	if outPath == "" {
		ext := filepath.Ext(inPath)
		outPath = inPath[:len(inPath)-len(ext)] + "-binaural.wav"
	}
	dst, err := spatial.CreateWAV(outPath, src.Rate, 2)
	if err != nil {
		return err
	}

	// Streaming, so a whole album never becomes resident. The buffered path
	// wanted frames x channels x 8 bytes, which is 6.4 GB for a 23 minute
	// track expanded to twelve channels.
	in := [][]float64{make([]float64, blockFrames), make([]float64, blockFrames)}
	outL := make([]float64, blockFrames)
	outR := make([]float64, blockFrames)
	stereo := [][]float64{outL, outR}
	gain := math.Pow(10, *gainDB/20)
	maxSample := 0.0

	for {
		n, err := src.Read(in)
		if err == io.EOF {
			break
		}
		if err != nil {
			dst.Close()
			return err
		}
		if err := pipe.Process(in[0][:n], in[1][:n], outL[:n], outR[:n]); err != nil {
			dst.Close()
			return err
		}
		for i := range n {
			outL[i] *= gain
			outR[i] *= gain
			maxSample = math.Max(maxSample, math.Max(math.Abs(outL[i]), math.Abs(outR[i])))
		}
		if err := dst.Write(stereo, n); err != nil {
			dst.Close()
			return err
		}
	}
	if err := dst.Close(); err != nil {
		return err
	}

	fmt.Printf("  peak     %.3f\n", maxSample)
	if maxSample > 1 {
		fmt.Println("  warning  the render clipped, try --gain -3")
	}
	fmt.Printf("  out      %s, 2 ch, 24-bit\n", filepath.Base(outPath))
	fmt.Printf("  took     %s\n", time.Since(started).Round(time.Millisecond))
	return nil
}

// blockFrames is the streaming block. Big enough that per-block overhead is
// irrelevant, small enough that the intermediate layout buffer stays tiny.
const blockFrames = 4096

// resolveTarget decides what layout to render, preferring an explicit upmix
// target, then an explicit input layout, then the channel count.
func resolveTarget(layoutName, upmixTo string, src *spatial.Reader) (spatial.Layout, error) {
	if upmixTo != "" {
		if src.Channels != 2 {
			return spatial.Layout{}, fmt.Errorf(
				"occam-spatial: --upmix takes stereo, this file has %d channels", src.Channels)
		}
		return spatial.LayoutByName(upmixTo)
	}
	if layoutName != "" {
		return spatial.LayoutByName(layoutName)
	}
	return spatial.LayoutForChannels(src.Channels)
}

func usage() {
	fmt.Fprint(os.Stderr, `occam-spatial renders multichannel audio to binaural stereo.

  occam-spatial [flags] <input.wav>

Nothing on macOS emits discrete surround channels, so the usual path is to
synthesise a layout from stereo and then render that:

  occam-spatial --upmix 7.1 track.wav

7.1 rather than 7.1.4 on purpose. The height channels cost eight extra
convolutions per sample and, without head tracking, listening tests here could
not tell them from the front pair. 7.1.4 is still available if you have a
reason to want it.

Already have multichannel audio? Just render it:

  occam-spatial movie-5.1.wav

Peak is reported rather than normalised: streaming means the loudest sample is
not known until the end. Use --gain to level match, and note that comparing a
render against its source needs loudness matching, not peak matching, since
binaural summing keeps peaks while dropping average energy.

Flags:
`)
	flag.PrintDefaults()
}
