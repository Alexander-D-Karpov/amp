package main

import (
	"os"
	"time"

	"github.com/gopxl/beep/mp3"
	"github.com/gordonklaus/portaudio" // PortAudio Go binding :contentReference[oaicite:3]{index=3}
)

func main() {
	f, _ := os.Open("/home/sanspie/Downloads/snowcone-deadmau5.mp3")
	stream, format, _ := mp3.Decode(f)
	defer stream.Close()

	portaudio.Initialize()
	defer portaudio.Terminate()

	out, _ := portaudio.OpenDefaultStream(
		0, 2, float64(format.SampleRate), // nIn, nOut, Hz
		format.SampleRate.N(time.Millisecond*20), // frames per buffer
		func(out [][]float32) {
			tmp := make([][2]float64, len(out[0])) // stereo
			n, _ := stream.Stream(tmp)
			for i := 0; i < n; i++ {
				out[0][i] = float32(tmp[i][0])
				out[1][i] = float32(tmp[i][1])
			}
		})
	out.Start()
	select {} // keep alive
}
