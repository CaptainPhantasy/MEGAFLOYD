Analyzes images for visual debugging, design-to-code workflows, and screenshot interpretation.

<when_to_use>
Use this tool when:
- User shares a screenshot and asks what's wrong
- Analyzing error messages captured in screenshots
- Converting design mockups to code
- Understanding diagrams, flowcharts, or architecture drawings
- Reading text from images (OCR-like functionality)
- User asks about an image file they have
</when_to_use>

<when_not_to_use>
Skip this tool when:
- User's request is purely text-based
- No image file path is provided
- The task doesn't require visual analysis
</when_not_to_use>

<parameters>
- `image_path` (required): The path to the image file to analyze. Can be absolute or relative to working directory.
- `prompt` (optional): A specific question or instruction about the image to focus the analysis.
</parameters>

<supported_formats>
- JPEG (.jpg, .jpeg)
- PNG (.png)
- GIF (.gif)
- WebP (.webp)
- BMP (.bmp)
</supported_formats>

<limitations>
- Maximum file size: 5MB
- Requires a vision-capable model (not all providers support images)
- Image is base64 encoded and sent to the model
</limitations>

<examples>
Analyze a screenshot for errors:
```json
{
  "image_path": "/Users/name/Desktop/error-screenshot.png",
  "prompt": "What error is displayed in this screenshot?"
}
```

Convert a design mockup to code:
```json
{
  "image_path": "./designs/login-page.png",
  "prompt": "Generate HTML and CSS for this login page design"
}
```

Understand a diagram:
```json
{
  "image_path": "./docs/architecture.png",
  "prompt": "Explain the architecture shown in this diagram"
}
```

Simple image analysis:
```json
{
  "image_path": "./screenshot.png"
}
```
</examples>

<tips>
- Use absolute paths when possible for clarity
- Include a specific prompt to get focused analysis
- For screenshots of errors, ask specifically about the error message
- For design mockups, specify what code you want generated
- The tool will fail gracefully if the model doesn't support images
</tips>

<errors>
- "image_path is required": You must provide an image path
- "Failed to access image file": The file doesn't exist or isn't readable
- "Image file too large": File exceeds 5MB limit
- "Unsupported image type": Use a supported format (.jpg, .png, .gif, .webp, .bmp)
- "The current model does not support image input": Switch to a vision-capable model
</errors>
