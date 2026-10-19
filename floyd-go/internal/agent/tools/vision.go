package tools

import (
	"context"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/legacy-ai/floyd/internal/permission"
)

//go:embed vision.md
var visionDescription []byte

const VisionToolName = "vision"

type VisionParams struct {
	ImagePath string `json:"image_path" description:"The path to the image file to analyze"`
	Prompt    string `json:"prompt,omitempty" description:"Optional question or instruction about the image"`
}

type VisionPermissionsParams struct {
	ImagePath string `json:"image_path"`
	Prompt    string `json:"prompt"`
}

type VisionResponseMetadata struct {
	ImagePath string `json:"image_path"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

func NewVisionTool(permissions permission.Service, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		VisionToolName,
		string(visionDescription),
		func(ctx context.Context, params VisionParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.ImagePath == "" {
				return fantasy.NewTextErrorResponse("image_path is required"), nil
			}

			// Resolve the image path
			imagePath := params.ImagePath
			if !filepath.IsAbs(imagePath) {
				imagePath = filepath.Join(workingDir, imagePath)
			}

			// Check if file exists
			info, err := os.Stat(imagePath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to access image file: %s", err)), nil
			}

			// Check file size (max 5MB for images)
			const maxImageSize = 5 * 1024 * 1024
			if info.Size() > maxImageSize {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Image file too large: %d bytes (max %d bytes)", info.Size(), maxImageSize)), nil
			}

			// Validate image type
			ext := strings.ToLower(filepath.Ext(imagePath))
			validTypes := map[string]string{
				".jpg":  "image/jpeg",
				".jpeg": "image/jpeg",
				".png":  "image/png",
				".gif":  "image/gif",
				".webp": "image/webp",
				".bmp":  "image/bmp",
			}
			mimeType, ok := validTypes[ext]
			if !ok {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Unsupported image type: %s. Supported types: .jpg, .jpeg, .png, .gif, .webp, .bmp", ext)), nil
			}

			// Request permission
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for vision tool")
			}

			granted, err := permissions.Request(ctx,
				permission.CreatePermissionRequest{
					SessionID:   sessionID,
					Path:        imagePath,
					ToolCallID:  call.ID,
					ToolName:    VisionToolName,
					Action:      "analyze",
					Description: fmt.Sprintf("Analyze image file: %s", imagePath),
					Params:      VisionPermissionsParams(params),
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(), nil
			}

			// Read and encode the image
			imageData, err := os.ReadFile(imagePath)
			if err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("Failed to read image file: %s", err)), nil
			}

			// Check if the provider supports images
			if !GetSupportsImagesFromContext(ctx) {
				// Provider doesn't support images - return text description request
				return fantasy.NewTextErrorResponse(
					"The current model does not support image input. " +
						"Please describe the image content in text instead."), nil
			}

			// Build the response with base64 encoded image
			base64Data := base64.StdEncoding.EncodeToString(imageData)

			metadata := VisionResponseMetadata{
				ImagePath: imagePath,
				MimeType:  mimeType,
				SizeBytes: info.Size(),
			}

			response := fantasy.NewImageResponse([]byte(base64Data), mimeType)
			if params.Prompt != "" {
				response.Content = fmt.Sprintf("Image: %s\n\nUser question: %s", filepath.Base(imagePath), params.Prompt)
			} else {
				response.Content = fmt.Sprintf("Image: %s", filepath.Base(imagePath))
			}

			return fantasy.WithResponseMetadata(response, metadata), nil
		})
}
