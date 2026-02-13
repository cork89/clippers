# Shader Preview Plan

## Overview

Add real-time shader preview to the segment editor in the web UI using WebGL. The shaders will run in the browser to preview effects before rendering the final video.

## Architecture

### Backend Changes

1. **Serve Shader Files**
   - Create endpoint `/api/shaders` to list available shaders
   - Create endpoint `/api/shaders/{name}` to serve individual GLSL files
   - Or: embed shader files at build time

2. **CORS Headers**
   - Ensure shader files are accessible from the web UI

### Frontend Changes

1. **Canvas Element**
   - Replace or overlay the current `<img>` in segment preview with `<canvas>`
   - Maintain the same dimensions and styling

2. **WebGL Renderer**
   - Create `ShaderPreview` class/module
   - Initialize WebGL context on canvas
   - Load image as texture
   - Compile and link shader programs
   - Render with uniforms

3. **Shader Loading**
   - Fetch GLSL files from backend
   - Adapt GLSL for WebGL compatibility (minor adjustments)
   - Cache compiled shaders

4. **Animation Loop**
   - Use `requestAnimationFrame` for animated shaders
   - Pass `u_time` uniform for animation
   - Handle play/pause for preview

## Implementation Details

### File Structure

```
internal/
  views/
    shader.go          # WebGL shader preview component
    static/
      shader.js        # WebGL rendering logic
      shaders/         # (or use existing shaders/ folder)
```

### Shader Adaptations

Your existing GLSL shaders use some features that need adjustment for WebGL:

1. **Vertex attributes** - Standardize on position and texcoord
2. **Uniforms** - Add `u_time`, `u_resolution`, `u_texture`
3. **Precision** - Add `precision mediump float;` to fragment shaders

### Example WebGL Flow

```javascript
// 1. Initialize
const gl = canvas.getContext('webgl');
gl.clearColor(0, 0, 0, 1);

// 2. Load shader
const program = compileShader(vertexSource, fragmentSource);

// 3. Load image texture
const texture = loadImageAsTexture(imageUrl);

// 4. Render loop
function render(time) {
  gl.uniform1f(uTimeLocation, time / 1000);
  gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
  requestAnimationFrame(render);
}
```

### API Endpoints

```
GET /api/shaders              # List available shaders
GET /api/shaders/{name}       # Get shader source
```

### UI Component

- Click shader option → loads and applies shader to canvas
- "None" option → shows original image without shader
- Play/pause button for animated effects
- Optional: slider to scrub through animation time

## Options

### Option 1: Static Preview (Simpler)

- Apply shader once, show static result
- Good for: pixel_melt, voronoi, retro (effects that don't animate)
- Not ideal for: wave_displace, liquid_flow (animated effects)

### Option 2: Animated Preview (Recommended)

- Full animation loop in browser
- Good for: all shaders, especially animated ones
- More complex: requires animation loop, time uniform handling

### Option 3: Hybrid

- Show static preview by default
- "Animate" toggle to enable animation
- Best of both worlds, moderate complexity

## Testing

- Test each shader renders without errors
- Verify image loads correctly as texture
- Check animation performance (should be smooth 60fps)
- Fallback to static if WebGL not supported

## Dependencies

- No new backend dependencies
- Frontend: pure WebGL (no libraries needed)
- Optionally: gl-matrix for matrix math if needed

## Future Enhancements

- Show/hide comparison (original vs shader)
- Adjustable shader parameters via UI sliders
- Export frame from canvas
