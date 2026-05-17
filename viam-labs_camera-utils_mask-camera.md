# Model viam-labs:camera-utils:mask-camera

`mask-camera` is a camera component that wraps another configured camera on the same machine and applies a PNG mask to its output. Pixels (and 3D points projected to pixel coordinates) that fall on a non-black region of the mask are passed through; everything else is filtered out.

This is useful for ignoring fixed areas of a camera's field of view, for example:
- Blocking out a region of the scene that always contains irrelevant motion.
- Restricting computer-vision processing to a region of interest.
- Removing parts of a point cloud that fall outside a known area.

## How the mask is interpreted

The mask is loaded from a PNG file at the path given by `mask_path`. For each pixel of the mask:

- If the sum of the red, green, and blue channels is greater than zero (i.e. the pixel is **not** pure black), the pixel is **visible** and the corresponding pixel from the base camera is kept.
- If the pixel is pure black (`#000000`), the corresponding pixel from the base camera is dropped. In returned `image.RGBA` images this results in a fully transparent pixel.

The mask's resolution should match the resolution of the base camera's images. Pixels outside the mask's bounds (and `NaN` coordinates from point cloud projection) are treated as masked-out.

## Behavior

- **`GetImages` / `Images`** — Calls the base camera's `Images`, then for every returned JPEG image produces a new RGBA image of the same dimensions where masked-out pixels are transparent. Source name and annotations are preserved. Non-JPEG images are currently skipped and will not appear in the response.
- **`NextPointCloud`** — Calls the base camera's `NextPointCloud` and uses the base camera's intrinsic properties to project each 3D point to pixel coordinates. Points whose projection lies on a visible mask pixel are kept; all others are dropped.
- **`Properties`** — Returned unchanged from the base camera.
- **`Stream`, `DoCommand`, `Geometries`, `SubscribeRTP`, `Unsubscribe`** — Not implemented; calling these returns an error.

## Configuration

The following attribute template can be used to configure this model:

```json
{
  "mask_path": "<string>",
  "camera": "<string>"
}
```

### Attributes

The following attributes are available for this model:

| Name        | Type   | Inclusion | Description                                                                                              |
|-------------|--------|-----------|----------------------------------------------------------------------------------------------------------|
| `mask_path` | string | Required  | Absolute path to a PNG image used as the mask. Non-black pixels are visible; black pixels are masked.    |
| `camera`    | string | Required  | Name of the base camera component on the same machine whose images and point clouds will be masked.      |

### Dependencies

The component declared in `camera` is a required dependency and must be configured on the same machine.

### Example Configuration

```json
{
  "mask_path": "/home/viam/masks/region_of_interest.png",
  "camera": "webcam-1"
}
```
