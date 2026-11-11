package native

import (
	"context"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"ottr/internal/library"
	"ottr/internal/native/kit"
	player "ottr/internal/player"
)

type playback struct {
	id          int64
	video       *ui.VideoPlayer
	audio       *audio.Player
	cancel      context.CancelFunc
	loading     bool
	err         string
	at          time.Time
	offset      time.Duration
	paused      bool
	seeking     bool
	seekSeconds float64
}

func (a *App) closePlayback() {
	a.playbackGen++
	p := a.playback
	a.playback = playback{}
	if p.cancel != nil {
		p.cancel()
	}
	if p.video != nil {
		p.video.Close()
	}
	if p.audio != nil {
		p.audio.Close()
	}
}

func (a *App) togglePlayback(m library.Media) {
	if a.playback.loading {
		return
	}
	if a.playback.id != m.ID || a.playback.video == nil && a.playback.audio == nil {
		a.openPlayback(m)
		return
	}
	if a.playbackPlaying() {
		a.playback.offset = a.playbackPosition(m)
		a.playback.paused = true
		if a.playback.video != nil {
			a.playback.video.Pause()
		}
		if a.playback.audio != nil {
			a.playback.audio.Pause()
		}
		return
	}
	if !a.playback.paused {
		a.playback.offset = 0 // replay after the end
	}
	a.playback.at = time.Now()
	a.playback.paused = false
	if a.playback.video != nil {
		a.playback.video.Play()
	}
	if a.playback.audio != nil {
		a.playback.audio.Play()
	}
}

func (a *App) openPlayback(m library.Media) {
	a.openPlaybackAt(m, 0, true)
}

func (a *App) seekPlayback(m library.Media, position time.Duration) {
	if a.playback.loading || m.DurationMs <= 0 {
		return
	}
	resume := true
	if a.playback.id == m.ID && (a.playback.video != nil || a.playback.audio != nil) {
		resume = a.playbackPlaying()
	}
	a.openPlaybackAt(m, position, resume)
}

func (a *App) openPlaybackAt(m library.Media, position time.Duration, resume bool) {
	a.closePlayback()
	duration := time.Duration(m.DurationMs) * time.Millisecond
	position = max(0, min(position, duration))
	ctx, cancel := context.WithCancel(context.Background())
	a.playbackGen++
	gen := a.playbackGen
	a.playback = playback{id: m.ID, cancel: cancel, loading: true, offset: position, seekSeconds: position.Seconds(), paused: !resume}
	go func() {
		var video *ui.VideoPlayer
		if m.HasVideo {
			decoder, err := player.OpenVideoAt(ctx, m.Path, m.Width, m.Height, position)
			if err != nil {
				cancel()
				a.update(func() { a.playbackFailed(gen, m.ID, err) })
				return
			}
			video, err = ui.NewVideoPlayer(decoder)
			if err != nil {
				decoder.Close()
				a.update(func() { a.playbackFailed(gen, m.ID, err) })
				return
			}
		}
		var sound *audio.Player
		if m.HasAudio {
			pcm, err := player.OpenAudio(ctx, m.Path, time.Duration(m.DurationMs)*time.Millisecond)
			if err == nil {
				sound = audio.NewPlayer(pcm)
				sound.Seek(position)
				if a.muted {
					audio.SetVolume(0)
				}
			} else {
				if video != nil {
					video.Close()
				}
				a.update(func() { a.playbackFailed(gen, m.ID, err) })
				return
			}
		}
		a.update(func() {
			if gen != a.playbackGen || a.sel != m.ID {
				if video != nil {
					video.Close()
				}
				if sound != nil {
					sound.Close()
				}
				return
			}
			a.playback = playback{id: m.ID, video: video, audio: sound, at: time.Now(), offset: position, seekSeconds: position.Seconds(), paused: !resume}
			if video != nil {
				if resume {
					video.Play()
				} else {
					video.Pause()
				}
			}
			if sound != nil && resume {
				sound.Play()
			}
		})
	}()
}

func (a *App) playbackFailed(gen int64, id int64, err error) {
	if gen != a.playbackGen {
		return
	}
	a.playback = playback{id: id, err: err.Error()}
}

func (a *App) playbackPosition(m library.Media) time.Duration {
	p := a.playback
	duration := time.Duration(m.DurationMs) * time.Millisecond
	if p.audio != nil {
		return min(p.audio.Position(), duration)
	}
	if p.paused {
		return min(p.offset, duration)
	}
	if p.video != nil && !p.video.Playing() || p.audio != nil && !p.audio.Playing() && !p.paused {
		return duration
	}
	if p.at.IsZero() {
		return min(p.offset, duration)
	}
	return min(p.offset+time.Since(p.at), duration)
}

func (a *App) playbackPlaying() bool {
	if a.playback.video != nil {
		return a.playback.video.Playing()
	}
	return a.playback.audio != nil && a.playback.audio.Playing()
}

// nativePlayerStage is the media on its dark ground, the seek bar split into
// the summary's chapters, and the transport under it.
func (a *App) nativePlayerStage(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	ui.Column(c).FillWidth().Gap(12).Children(func() {
		ratio := float32(16.0 / 9.0)
		if m.Width > 0 && m.Height > 0 {
			ratio = float32(m.Width) / float32(m.Height)
		}
		stage := ui.Column(c).FillWidth().AspectRatio(ratio).Radius(kit.RadiusCard).Clip().
			Background(p.Media).Center()
		stage.Children(func() {
			switch {
			case a.playback.id == m.ID && a.playback.video != nil:
				ui.Video(c, a.playback.video).Fill().Fit(ui.Contain)
			case m.Source == "screenshot":
				if picture := a.thumbs[m.ID]; picture != nil {
					ui.Image(c, picture).Fit(ui.Contain).Fill()
				} else {
					ui.Icon(c, iconImage).FontSize(28).TextColor(onMedia()).Opacity(0.6)
				}
			case !m.HasVideo:
				ui.Icon(c, iconWaveform).FontSize(28).TextColor(onMedia()).Opacity(0.6)
			case a.playback.loading && a.playback.id == m.ID:
				ui.Column(c).Center().Gap(8).Children(func() {
					ui.Spinner(c).Label(a.tr("Loading"))
					ui.Text(c, a.tr("Loading…")).FontSize(12).TextColor(onMedia())
				})
			default:
				if picture := a.thumbs[m.ID]; picture != nil {
					ui.Image(c, picture).Fit(ui.Contain).Fill()
				}
				play := ui.ButtonBase(c).Label(a.tr("Play")).Tooltip(a.tr("Play in Ottr")).Size(56, 56).
					Radius(28).Center().Background(ui.RGBA(255, 255, 255, 0.14)).Cursor(ui.CursorPointer)
				if a.thumbs[m.ID] != nil {
					play.Attach(ui.AnchorCenter, ui.AnchorCenter)
				}
				play.Children(func() { ui.Icon(c, iconPlay).FontSize(24).TextColor(onMedia()) })
				if play.Hovered() {
					play.Background(ui.RGBA(255, 255, 255, 0.24))
				}
				if play.Clicked() {
					a.togglePlayback(m)
				}
			}
		})
		if a.playback.id == m.ID && a.playback.err != "" {
			ui.Text(c, a.playback.err).FontSize(11.5).TextColor(p.Ink3).SingleLine().MinWidth(0).
				Tooltip(a.playback.err)
		}
		if m.Source != "screenshot" {
			a.playbackSeekSlider(c, t, m)
		}
		a.playbackControls(c, t, m)
	})
}

// playbackControls is the transport: play, ten seconds back and on, the clock,
// and on the right the translation under lines, mute, full screen and more.
func (a *App) playbackControls(c *ui.Context, t *ui.Theme, m library.Media) {
	p := pal(t)
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
		if m.Source == "screenshot" {
			ui.Text(c, a.tr("Still image")).FontSize(12).TextColor(p.Ink3)
			return
		}
		label, icon := a.tr("Play"), iconPlay
		if a.playback.id == m.ID && a.playbackPlaying() {
			label, icon = a.tr("Pause"), iconPause
		}
		play := ui.ButtonBase(c).Label(label).Tooltip(label).Size(36, 36).Radius(18).Center().
			Background(p.Btn).TextColor(p.BtnInk).Cursor(ui.CursorPointer)
		play.Children(func() { ui.Icon(c, icon).FontSize(16) })
		if play.Clicked() {
			a.togglePlayback(m)
		}
		ui.Box(c).Width(4)
		kit.IconButton(c, t, iconSkipBack, a.tr("Back 10 seconds"), func() {
			a.seekPlayback(m, a.playbackPosition(m)-10*time.Second)
		})
		kit.IconButton(c, t, iconSkipForward, a.tr("Forward 10 seconds"), func() {
			a.seekPlayback(m, a.playbackPosition(m)+10*time.Second)
		})
		ui.Row(c).Gap(6).Padding(0, 8).AlignItems(ui.Center).Shrink(0).Children(func() {
			mono(ui.Text(c, fmtDuration(a.playbackPosition(m).Milliseconds()))).FontSize(13).
				FontWeight(500).TextColor(p.Ink)
			ui.Text(c, "/").FontSize(13).TextColor(p.Ink4)
			mono(ui.Text(c, fmtDuration(m.DurationMs))).FontSize(13).TextColor(p.Ink3)
		})
		ui.Spacer(c)
		if len(a.segs) > 0 {
			captions := kit.IconButton(c, t, iconCaptions, a.tr("Show translation"), func() {
				a.setTranslationVisible(m, !a.showTranslation)
			})
			if a.showTranslation {
				captions.Background(p.Hover).TextColor(p.Ink)
			}
		}
		volumeIcon := iconSpeaker
		if a.muted {
			volumeIcon = iconVolumeX
		}
		kit.IconButton(c, t, volumeIcon, a.tr("Mute"), a.toggleMute)
		kit.IconButton(c, t, iconFullscreen, a.tr("Enter full screen"), func() {
			if a.win != nil {
				a.win.ToggleFullScreen()
			}
		})
		more := kit.IconButton(c, t, iconMoreH, a.tr("More actions"), nil)
		more.Menu(func(mn *ui.Menu) {
			if mn.Item(a.tr("Show the file")).Chosen() {
				mygo.Shell.ShowItemInFolder(m.Path)
			}
		})
	})
}

func (a *App) toggleMute() {
	a.muted = !a.muted
	if a.muted {
		audio.SetVolume(0)
	} else {
		audio.SetVolume(1)
	}
}

// playbackSeekSlider is the seek bar: the summary's chapters as segments with
// gaps between them, what has played in ink, teal ticks where the search
// found something, and the teal playhead. Changed moves the thumb at once;
// release seeks.
func (a *App) playbackSeekSlider(c *ui.Context, t *ui.Theme, m library.Media) {
	if m.DurationMs <= 0 {
		return
	}
	p := pal(t)
	if !a.playback.seeking {
		a.playback.seekSeconds = a.playbackPosition(m).Seconds()
	}
	duration := float64(m.DurationMs) / 1000
	slider := ui.SliderBase(c, &a.playback.seekSeconds, 0, duration).
		Label(a.tr("Playback position")).FillWidth().MinWidth(60).Height(16).Cursor(ui.CursorPointer)
	fraction := float32(a.playback.seekSeconds / duration)
	var cuts []float32
	for _, ch := range a.sum.Chapters {
		if ch.StartMs > 0 && ch.StartMs < m.DurationMs {
			cuts = append(cuts, float32(ch.StartMs)/float32(m.DurationMs))
		}
	}
	var ticks []float32
	for _, id := range a.searchHits() {
		for _, seg := range a.segs {
			if seg.ID == id {
				ticks = append(ticks, float32(seg.StartMs)/float32(m.DurationMs))
			}
		}
	}
	slider.Draw(func(pt *ui.Painter, r ui.Rect) {
		y := r.Y + r.H/2 - 1.5
		bounds := append([]float32{0}, cuts...)
		bounds = append(bounds, 1)
		for i := 0; i+1 < len(bounds); i++ {
			x0, x1 := r.X+r.W*bounds[i], r.X+r.W*bounds[i+1]
			if i > 0 {
				x0 += 1.5
			}
			if i+2 < len(bounds) {
				x1 -= 1.5
			}
			if x1 <= x0 {
				continue
			}
			pt.Fill(ui.Rect{X: x0, Y: y, W: x1 - x0, H: 3}, p.Line2, 1.5)
			played := min(r.X+r.W*fraction, x1)
			if played > x0 {
				pt.Fill(ui.Rect{X: x0, Y: y, W: played - x0, H: 3}, p.Ink, 1.5)
			}
		}
		for _, tick := range ticks {
			pt.Fill(ui.Rect{X: r.X + r.W*tick - 1, Y: r.Y + r.H/2 - 5, W: 2, H: 10}, p.Accent, 1)
		}
		head := ui.Rect{X: r.X + r.W*fraction - 6, Y: r.Y + r.H/2 - 6, W: 12, H: 12}
		pt.Fill(head, p.Sheet, 6)
		pt.Fill(ui.Rect{X: head.X + 2, Y: head.Y + 2, W: 8, H: 8}, p.Accent, 4)
		pt.Stroke(head, p.Accent, 6, 1)
	})
	if slider.Changed() {
		a.playback.seeking = true
	}
	if a.playback.seeking && !slider.Pressed() {
		a.seekPlayback(m, time.Duration(a.playback.seekSeconds*float64(time.Second)))
	}
}
