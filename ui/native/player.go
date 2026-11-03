package native

import (
	"context"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"ottr/internal/library"
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

func (a *App) nativePlayerStage(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Column(c).FillWidth().Clip().
		BorderWidth(1).BorderColor(t.Border).Children(func() {
		from, to := mediaBackdrop(t.Dark)
		ratio := float32(16.0 / 9.0)
		if m.Width > 0 && m.Height > 0 {
			ratio = float32(m.Width) / float32(m.Height)
		}
		stage := ui.Column(c).FillWidth().AspectRatio(ratio).Gradient(from, to, 90).Center()
		stage.Children(func() {
			switch {
			case a.playback.id == m.ID && a.playback.video != nil:
				ui.Video(c, a.playback.video).Grow(1).Fit(ui.Contain)
			case m.Source == "screenshot":
				ui.Icon(c, iconScreen).FontSize(t.FontSize + 16).TextColor(onMedia()).Opacity(0.7)
			case !m.HasVideo:
				ui.Icon(c, iconMic).FontSize(t.FontSize + 16).TextColor(onMedia()).Opacity(0.7)
			case a.playback.loading && a.playback.id == m.ID:
				ui.Column(c).Center().Gap(t.Space(2)).Children(func() {
					ui.Spinner(c).Label(a.tr("Loading"))
					ui.Text(c, a.tr("Loading…")).FontSize(t.FontSize - 2).TextColor(onMedia())
				})
			default:
				play := ui.ButtonBase(c).Label(a.tr("Play")).Tooltip(a.tr("Play in Ottr")).
					Padding(t.Space(4)).Radius(t.Space(10)).Background(ui.RGBA(255, 255, 255, 0.12))
				play.Children(func() { ui.Icon(c, iconPlay).FontSize(t.FontSize + 12).TextColor(onMedia()) })
				if play.Hovered() {
					play.Background(ui.RGBA(255, 255, 255, 0.2))
				}
				if play.Clicked() {
					a.togglePlayback(m)
				}
			}
		})
		a.playbackControls(c, t, m)
	})
}

func (a *App) playbackControls(c *ui.Context, t *ui.Theme, m library.Media) {
	ui.Column(c).Background(playerBar(t)).Children(func() {
		ui.Row(c).Padding(t.Space(2), t.Space(3)).Gap(t.Space(3)).AlignItems(ui.Center).Children(func() {
			if m.Source != "screenshot" {
				label := a.tr("Play")
				icon := iconPlay
				if a.playback.id == m.ID && a.playbackPlaying() {
					label, icon = a.tr("Pause"), iconPause
				}
				btn := ui.ButtonBase(c).Label(label).Tooltip(label).Gap(t.Space(1.5)).TextColor(onMedia()).Shrink(0)
				btn.Children(func() {
					ui.Icon(c, icon).FontSize(t.FontSize + 1)
					ui.Text(c, label).FontSize(t.FontSize - 2)
				})
				if btn.Clicked() {
					a.togglePlayback(m)
				}
				a.playbackSeekSlider(c, t, m)
			} else {
				ui.Icon(c, iconScreen).FontSize(t.FontSize + 1).TextColor(onMedia())
			}
			ui.Text(c, fmtDuration(a.playbackPosition(m).Milliseconds())+" / "+fmtDuration(m.DurationMs)).
				FontSize(t.FontSize - 2).TextColor(onMedia()).Opacity(0.9).FontFeatures("tnum").SingleLine().Shrink(0)
		})
		note := ""
		switch {
		case m.Source == "screenshot":
			note = a.tr("Still image")
		case a.playback.id == m.ID && a.playback.err != "":
			note = a.playback.err
		}
		if note != "" {
			ui.Row(c).Padding(0, t.Space(3), t.Space(2)).Children(func() {
				ui.Text(c, note).FontSize(t.FontSize - 3).TextColor(onMedia()).Opacity(0.8).
					SingleLine().MinWidth(0).Tooltip(note)
			})
		}
	})
}

// playbackSeekSlider draws its own high-contrast track because the player bar
// is dark in both themes. Changed updates the thumb immediately; release seeks.
func (a *App) playbackSeekSlider(c *ui.Context, t *ui.Theme, m library.Media) {
	if m.DurationMs <= 0 {
		return
	}
	if !a.playback.seeking {
		a.playback.seekSeconds = a.playbackPosition(m).Seconds()
	}
	duration := float64(m.DurationMs) / 1000
	slider := ui.SliderBase(c, &a.playback.seekSeconds, 0, duration).
		Label(a.tr("Playback position")).Grow(1).MinWidth(60).Height(20).PaddingX(7)
	fraction := float32(a.playback.seekSeconds / duration)
	slider.Draw(func(p *ui.Painter, r ui.Rect) {
		const inset = float32(7)
		x, width := r.X+inset, r.W-2*inset
		y := r.Y + r.H/2 - 2
		p.Fill(ui.Rect{X: x, Y: y, W: width, H: 4}, ui.RGBA(255, 255, 255, 0.22), 2)
		p.Fill(ui.Rect{X: x, Y: y, W: width * fraction, H: 4}, onMedia(), 2)
		p.Fill(ui.Rect{X: x + width*fraction - 6, Y: r.Y + r.H/2 - 6, W: 12, H: 12}, onMedia(), 6)
	})
	if slider.Changed() {
		a.playback.seeking = true
	}
	if a.playback.seeking && !slider.Pressed() {
		a.seekPlayback(m, time.Duration(a.playback.seekSeconds*float64(time.Second)))
	}
}
