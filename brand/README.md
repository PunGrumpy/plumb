<!-- contentType: Reference · plan: docs/content-plan.md -->

# plumb brand assets

This page lists the plumb logo files, what the shapes mean, and the rules for using them. Open `preview.html` in a browser to see every asset on white and on black.

## What the mark means

The mark is a plumb bob, the weight that builders hang from a string to find true vertical. It matches what plumb does: follow a VM straight down through every layer of the cloud, from the API to the datapath.

The mark has only 2 shapes: the string and a solid triangle that points down. The single-color solid triangle comes from Vercel. The gap between the string and the bob keeps the mark from reading as a download arrow.

## Files

| File | Use it when |
| --- | --- |
| `mark.svg` | You embed it in HTML and want it to take the parent element's `color`. The file uses `currentColor` |
| `mark-black.svg`, `mark-white.svg` | You need an image file for a light or dark background |
| `wordmark.svg`, `wordmark-black.svg`, `wordmark-white.svg` | You show the mark with the name. The letters are paths in the file, so no font needs to be installed |
| `icon.svg`, `icon-512.png`, `icon-1024.png` | You need an app icon, or a picture for the repo or a chat bot. White mark on a black rounded square |
| `favicon.svg` | You need the website favicon. It turns white on its own when the system uses dark mode. The string is thicker than in `mark.svg` so it stays visible at 16 px |
| `og.svg`, `og.png` | You need a social card for link previews, at 2400 × 1350. Like vercel.com, it has no text: a black plumb bob on black, lit from behind so its edges glow, with a light grain |
| `preview.html` | You review the assets on one page |

## Colors

Like Vercel, plumb uses only black and white. Other colors are only for statuses in the CLI and never go on the logo.

| Name | Value | Use for |
| --- | --- | --- |
| Black | `#000000` | Mark, wordmark, and the background of icon and social card |
| White | `#FFFFFF` | Mark on black |
| Gray 400 | `#A1A1A1` | Secondary text on black |

## Fonts

The wordmark is drawn with a 7-unit stroke on a 30-unit x-height, so it replaces the letters without depending on a font. For other text, use Geist if you have it, then fall back to Inter and the system font. For code and commands, use Geist Mono, then fall back to SF Mono and Menlo.

## Clear space and minimum size

Leave space around the mark and the wordmark at least as long as the string. The minimum size of each file is:

| File     | Minimum size |
| -------- | ------------ |
| Mark     | 24 px        |
| Wordmark | 96 px        |
| Icon     | 32 px        |
| Favicon  | 16 px        |

If you need anything smaller, use `favicon.svg`.

## Don'ts

Each of these makes the mark stop reading as a plumb bob, or breaks its plain single color.

- Don't rotate the mark or flip it so the bob points up.
- Don't add colors or gradients.
- Don't stretch or squash it.
- Don't join the string to the triangle, because the mark turns into an arrow.
- Don't add shapes, dividers or shadows.
- Don't place the mark on a pattern that hides the gap between the string and the bob.
- Don't type the word plumb in another font in place of the wordmark.

## Edit and export again

The mark's shapes live in `mark.svg`, and the other files use the same coordinates. If you change the mark, change every file that contains it. PNG files are made from the SVGs with headless Chrome. This command makes `og.png` at 2400 × 1350:

```sh
chrome --headless --force-device-scale-factor=2 \
	--window-size=1200,675 --screenshot=og.png og.html
```

`og.html` is an HTML page with `<img src="og.svg">` at 1200 × 675 and a margin of 0. For the icons, use `--force-device-scale-factor=1` for 512 and `2` for 1024. The light and grain in `og.svg` come from SVG filters, so the result differs slightly between browsers. Publish `og.png`.
