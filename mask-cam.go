package camerautils

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"os"

	"github.com/golang/geo/r3"
	camera "go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/components/camera/rtppassthrough"
	"go.viam.com/rdk/gostream"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/spatialmath"
	"go.viam.com/rdk/utils"
)

var (
	MaskCamera       = resource.NewModel("viam-labs", "camera-utils", "mask-camera")
	errUnimplemented = errors.New("unimplemented")
)

func init() {
	resource.RegisterComponent(camera.API, MaskCamera,
		resource.Registration[camera.Camera, *Config]{
			Constructor: newCameraUtilsMaskCamera,
		},
	)
}

type Config struct {
	// Mask image (png)
	MaskPath string `json:"mask_path"`
	Camera   string `json:"camera"`
}

// Validate ensures all parts of the config are valid and important fields exist.
// Returns three values:
//  1. Required dependencies: other resources that must exist for this resource to work.
//  2. Optional dependencies: other resources that may exist but are not required.
//  3. An error if any Config fields are missing or invalid.
//
// The `path` parameter indicates
// where this resource appears in the machine's JSON configuration
// (for example, "components.0"). You can use it in error messages
// to indicate which resource has a problem.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if cfg.MaskPath == "" {
		return nil, nil, fmt.Errorf("%s: missing required field 'mask_path'", path)
	}
	if cfg.Camera == "" {
		return nil, nil, fmt.Errorf("%s: missing required field 'camera'", path)
	}
	return []string{cfg.Camera}, nil, nil
}

type maskGrid struct {
	w, h int
	bits [][]bool
}

func (g *maskGrid) showPixel(x, y float64) bool {
	if math.IsNaN(x) || math.IsNaN(y) {
		return false
	}
	if int(x) < 0 || int(x) >= g.w || int(y) < 0 || int(y) >= g.h {
		return false
	}
	return g.bits[int(y)][int(x)]
}

type cameraUtilsMaskCamera struct {
	resource.AlwaysRebuild
	resource.Named

	name resource.Name

	baseCamera camera.Camera
	mask       maskGrid

	logger logging.Logger
	cfg    *Config

	cancelCtx  context.Context
	cancelFunc func()
}

func loadMask(path string) (maskGrid, error) {
	f, err := os.Open(path)
	if err != nil {
		return maskGrid{}, err
	}
	defer f.Close()
	image, _, err := image.Decode(f)
	if err != nil {
		return maskGrid{}, err
	}
	b := image.Bounds()
	g := maskGrid{w: b.Dx(), h: b.Dy(), bits: make([][]bool, b.Dy())}

	for y := 0; y < g.h; y++ {
		g.bits[y] = make([]bool, g.w)
		for x := 0; x < g.w; x++ {
			red, green, blue, _ := image.At(b.Min.X+x, b.Min.Y+y).RGBA()
			g.bits[y][x] = (red | green | blue) > 0
		}
	}
	return g, nil
}

func newCameraUtilsMaskCamera(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (camera.Camera, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	return NewMaskCamera(ctx, deps, rawConf.ResourceName(), conf, logger)

}

func NewMaskCamera(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (camera.Camera, error) {

	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	baseCamera, err := camera.FromProvider(deps, conf.Camera)
	if err != nil {
		cancelFunc()
		return nil, err
	}

	mask, err := loadMask(conf.MaskPath)
	if err != nil {
		cancelFunc()
		return nil, err
	}

	s := &cameraUtilsMaskCamera{
		name:       name,
		baseCamera: baseCamera,
		mask:       mask,
		logger:     logger,
		cfg:        conf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}
	return s, nil
}

func (s *cameraUtilsMaskCamera) Name() resource.Name {
	return s.name
}

func (s *cameraUtilsMaskCamera) Stream(ctx context.Context, errHandlers ...gostream.ErrorHandler) (gostream.VideoStream, error) {
	var videoStreamRetVal gostream.VideoStream

	return videoStreamRetVal, fmt.Errorf("not implemented")
}

// Images is used for getting simultaneous images from different imagers from 3D cameras along with associated metadata,
// and single images from non-3D cameras e.g. webcams, RTSP cameras etc. in the NamedImage list response.
// It is not for getting a time series of images from the same imager.
//
// The filterSourceNames parameter can be used to filter only the images from the specified source names, and
// when unspecified, all images are returned.
//
// The extra parameter can be used to pass additional options to the camera resource.
func (s *cameraUtilsMaskCamera) Images(ctx context.Context, filterSourceNames []string, extra map[string]interface{}) ([]camera.NamedImage, resource.ResponseMetadata, error) {
	var responseMetadataRetVal resource.ResponseMetadata

	baseImages, responseMetadata, err := s.baseCamera.Images(ctx, filterSourceNames, extra)
	if err != nil {
		return nil, responseMetadataRetVal, err
	}
	responseMetadataRetVal = responseMetadata
	responseImages := make([]camera.NamedImage, 0, len(baseImages))

	for _, baseImage := range baseImages {

		if baseImage.MimeType() != utils.MimeTypeJPEG {
			//todo: support other image types
			continue
		}

		baseI, err := baseImage.Image(ctx)
		if err != nil {
			return nil, responseMetadataRetVal, err
		}
		w, h := baseI.Bounds().Dx(), baseI.Bounds().Dy()
		newImage := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if s.mask.showPixel(float64(x), float64(y)) {
					newImage.Set(x, y, baseI.At(x, y))
				}
			}
		}
		newNamedImage, err := camera.NamedImageFromImage(newImage, baseImage.SourceName, baseImage.MimeType(), baseImage.Annotations)
		if err != nil {
			return nil, responseMetadataRetVal, err
		}
		responseImages = append(responseImages, newNamedImage)
	}

	return responseImages, responseMetadataRetVal, nil
}

// NextPointCloud returns the next immediately available point cloud, not necessarily one
// a part of a sequence. In the future, there could be streaming of point clouds.
func (s *cameraUtilsMaskCamera) NextPointCloud(ctx context.Context, extra map[string]interface{}) (pointcloud.PointCloud, error) {
	pointCloudRetVal := pointcloud.NewBasicEmpty()

	properties, err := s.baseCamera.Properties(ctx)
	if err != nil {
		return nil, err
	}

	basePointCloud, err := s.baseCamera.NextPointCloud(ctx, extra)
	if err != nil {
		return nil, err
	}

	basePointCloud.Iterate(0, 0, func(p r3.Vector, d pointcloud.Data) bool {
		px, py, err := properties.PointToPixel(p)
		if err != nil {
			return true
		}
		if s.mask.showPixel(px, py) {
			pointCloudRetVal.Set(p, d)
			return true
		}
		return true
	})

	return pointCloudRetVal, nil
}

// Properties returns properties that are intrinsic to the particular
// implementation of a camera.
func (s *cameraUtilsMaskCamera) Properties(ctx context.Context) (camera.Properties, error) {
	return s.baseCamera.Properties(ctx)
}

func (s *cameraUtilsMaskCamera) Status(ctx context.Context) (map[string]interface{}, error) {
	return s.baseCamera.Status(ctx)
}

func (s *cameraUtilsMaskCamera) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *cameraUtilsMaskCamera) Geometries(ctx context.Context, extra map[string]interface{}) ([]spatialmath.Geometry, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *cameraUtilsMaskCamera) SubscribeRTP(ctx context.Context, bufferSize int, packetsCB rtppassthrough.PacketCallback) (rtppassthrough.Subscription, error) {
	var subscriptionRetVal rtppassthrough.Subscription

	return subscriptionRetVal, fmt.Errorf("not implemented")
}

func (s *cameraUtilsMaskCamera) Unsubscribe(ctx context.Context, id rtppassthrough.SubscriptionID) error {
	return fmt.Errorf("not implemented")
}

func (s *cameraUtilsMaskCamera) Close(context.Context) error {
	// Put close code here
	s.cancelFunc()
	return nil
}
