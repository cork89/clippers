// ./internal/views/static/shader.js

class ShaderPreview {
  constructor(canvasId) {
    this.canvas = document.getElementById(canvasId);
    if (!this.canvas) {
      console.error('Canvas not found:', canvasId);
      return;
    }
    
    this.gl = this.canvas.getContext('webgl') || this.canvas.getContext('experimental-webgl');
    if (!this.gl) {
      console.error('WebGL not supported');
      return;
    }
    
    this.program = null;
    this.texture = null;
    this.image = null;
    this.animationId = null;
    this.isPlaying = false;
    this.startTime = Date.now();
    this.currentShader = null;
    this.uniforms = {};
    this.attributeLocations = {};
    
    this.init();
  }
  
  init() {
    const gl = this.gl;
    
    // Set clear color
    gl.clearColor(0, 0, 0, 1);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
  }
  
  async loadShader(name) {
    if (name === 'none' || !name) {
      this.cleanup();
      return;
    }
    
    try {
      const [vertexSrc, fragmentSrc] = await Promise.all([
        fetch('/api/shaders/vertex').then(r => r.text()),
        fetch(`/api/shaders/${name}`).then(r => r.text())
      ]);
      
      this.compileProgram(vertexSrc, fragmentSrc);
      this.currentShader = name;
      
      if (this.image && this.image.complete) {
        this.loadTexture();
        if (this.isPlaying) {
          this.render();
        }
      }
    } catch (e) {
      console.error('Failed to load shader:', e);
    }
  }
  
  compileProgram(vertexSrc, fragmentSrc) {
    const gl = this.gl;
    
    const vertexShader = this.compileShader(gl.VERTEX_SHADER, vertexSrc);
    const fragmentShader = this.compileShader(gl.FRAGMENT_SHADER, fragmentSrc);
    
    if (!vertexShader || !fragmentShader) return;
    
    const program = gl.createProgram();
    gl.attachShader(program, vertexShader);
    gl.attachShader(program, fragmentShader);
    gl.linkProgram(program);
    
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      console.error('Program link error:', gl.getProgramInfoLog(program));
      return;
    }
    
    if (this.program) {
      gl.deleteProgram(this.program);
    }
    
    this.program = program;
    gl.useProgram(program);
    
    // Get attribute locations
    this.attributeLocations = {
      position: gl.getAttribLocation(program, 'a_position'),
      texCoord: gl.getAttribLocation(program, 'a_texCoord')
    };
    
    // Get uniform locations
    this.uniforms = {
      time: gl.getUniformLocation(program, 'u_time'),
      resolution: gl.getUniformLocation(program, 'u_resolution'),
      texture: gl.getUniformLocation(program, 'u_texture')
    };
    
    // Set up geometry
    this.setupGeometry();
  }
  
  compileShader(type, source) {
    const gl = this.gl;
    const shader = gl.createShader(type);
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      console.error('Shader compile error:', gl.getShaderInfoLog(shader));
      gl.deleteShader(shader);
      return null;
    }
    
    return shader;
  }
  
  setupGeometry() {
    const gl = this.gl;
    
    // Full screen quad
    const positions = new Float32Array([
      -1, -1,
       1, -1,
      -1,  1,
       1,  1
    ]);
    
    const texCoords = new Float32Array([
      0, 1,
      1, 1,
      0, 0,
      1, 0
    ]);
    
    // Position buffer
    const positionBuffer = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, positionBuffer);
    gl.bufferData(gl.ARRAY_BUFFER, positions, gl.STATIC_DRAW);
    
    if (this.attributeLocations.position >= 0) {
      gl.enableVertexAttribArray(this.attributeLocations.position);
      gl.vertexAttribPointer(this.attributeLocations.position, 2, gl.FLOAT, false, 0, 0);
    }
    
    // TexCoord buffer
    const texCoordBuffer = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, texCoordBuffer);
    gl.bufferData(gl.ARRAY_BUFFER, texCoords, gl.STATIC_DRAW);
    
    if (this.attributeLocations.texCoord >= 0) {
      gl.enableVertexAttribArray(this.attributeLocations.texCoord);
      gl.vertexAttribPointer(this.attributeLocations.texCoord, 2, gl.FLOAT, false, 0, 0);
    }
  }
  
  loadTexture() {
    if (!this.image || !this.gl) return;
    
    const gl = this.gl;
    
    if (this.texture) {
      gl.deleteTexture(this.texture);
    }
    
    const texture = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, texture);
    
    // Set texture parameters
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    
    // Upload the image
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, this.image);
    
    this.texture = texture;
  }
  
  setImage(imageElement) {
    this.image = imageElement;
    
    if (this.image && this.image.complete) {
      this.loadTexture();
    } else if (this.image) {
      this.image.onload = () => {
        this.loadTexture();
      };
    }
  }
  
  render() {
    if (!this.program || !this.texture || !this.canvas) return;
    
    const gl = this.gl;
    
    gl.viewport(0, 0, this.canvas.width, this.canvas.height);
    gl.clear(gl.COLOR_BUFFER_BIT);
    
    gl.useProgram(this.program);
    
    // Set uniforms
    const time = (Date.now() - this.startTime) / 1000;
    gl.uniform1f(this.uniforms.time, time);
    gl.uniform2f(this.uniforms.resolution, this.canvas.width, this.canvas.height);
    gl.uniform1i(this.uniforms.texture, 0);
    
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
  }
  
  start() {
    if (this.isPlaying) return;
    
    this.isPlaying = true;
    this.startTime = Date.now();
    
    const animate = () => {
      if (!this.isPlaying) return;
      this.render();
      this.animationId = requestAnimationFrame(animate);
    };
    
    animate();
  }
  
  stop() {
    this.isPlaying = false;
    if (this.animationId) {
      cancelAnimationFrame(this.animationId);
      this.animationId = null;
    }
  }
  
  toggle() {
    if (this.isPlaying) {
      this.stop();
    } else {
      this.start();
    }
    return this.isPlaying;
  }
  
  cleanup() {
    this.stop();
    
    if (this.program) {
      this.gl.deleteProgram(this.program);
      this.program = null;
    }
    
    if (this.texture) {
      this.gl.deleteTexture(this.texture);
      this.texture = null;
    }
    
    this.currentShader = null;
  }
  
  resize() {
    if (!this.canvas || !this.image) return;
    
    // Don't resize if already correct size
    const targetWidth = this.image.naturalWidth;
    const targetHeight = this.image.naturalHeight;
    
    if (this.canvas.width === targetWidth && this.canvas.height === targetHeight) {
      return;
    }
    
    const container = this.canvas.parentElement;
    const maxWidth = container.clientWidth;
    const maxHeight = container.clientHeight;
    
    const imgRatio = this.image.naturalWidth / this.image.naturalHeight;
    const containerRatio = maxWidth / maxHeight;
    
    let width, height;
    if (imgRatio > containerRatio) {
      width = maxWidth;
      height = maxWidth / imgRatio;
    } else {
      height = maxHeight;
      width = maxHeight * imgRatio;
    }
    
    this.canvas.width = targetWidth;
    this.canvas.height = targetHeight;
    this.canvas.style.width = `${width}px`;
    this.canvas.style.height = `${height}px`;
  }
}

// Global registry for shader previews
const shaderPreviews = {};

function initShaderPreview(segmentIndex, canvasId, imageSrc, initialShader) {
  const canvas = document.getElementById(canvasId);
  if (!canvas) return;
  
  // Create hidden image element to load texture
  const img = new Image();
  img.crossOrigin = 'anonymous';
  img.onload = () => {
    if (shaderPreviews[segmentIndex]) {
      shaderPreviews[segmentIndex].resize();
      if (shaderPreviews[segmentIndex].currentShader) {
        shaderPreviews[segmentIndex].render();
      }
    }
  };
  img.src = imageSrc;
  
  // Create preview instance
  const preview = new ShaderPreview(canvasId);
  preview.setImage(img);
  
  // Wait for image to load then set up
  img.onload = () => {
    preview.setImage(img);
    preview.resize();
    
    // Load initial shader if set
    if (initialShader && initialShader !== 'none') {
      preview.loadShader(initialShader).then(() => {
        preview.render();
      });
    }
  };
  
  // Handle resize
  window.addEventListener('resize', () => {
    if (shaderPreviews[segmentIndex]) {
      shaderPreviews[segmentIndex].resize();
    }
  });
  
  shaderPreviews[segmentIndex] = preview;
  
  return preview;
}

function loadShaderForSegment(segmentIndex, shaderName) {
  const preview = shaderPreviews[segmentIndex];
  if (!preview) return;
  
  if (shaderName === 'none' || !shaderName) {
    preview.cleanup();
    preview.stop();
    return;
  }
  
  preview.loadShader(shaderName).then(() => {
    preview.start();
  });
}

function toggleShaderAnimation(segmentIndex) {
  const preview = shaderPreviews[segmentIndex];
  if (!preview) return;
  
  const isPlaying = preview.toggle();
  
  // Update button state if exists
  const btn = document.querySelector(`[data-segment="${segmentIndex}"].shader-play-btn`);
  if (btn) {
    btn.textContent = isPlaying ? '⏸' : '▶';
  }
}

// Auto-initialize on htmx swap
document.addEventListener('htmx:afterSwap', (event) => {
  const editorContent = event.target.closest('#editor-panel-content');
  if (!editorContent) return;
  
  const img = editorContent.querySelector('#segment-preview-image');
  if (!img) return;
  
  // Get shader from data attribute (set from project settings)
  const shader = editorContent.dataset.shader || 'none';
  
  // Initialize shader preview if shader is not 'none'
  if (shader && shader !== 'none' && !shaderPreviews[0]) {
    initShaderPreview(0, 'shader-canvas', img.src, shader);
    
    // Show canvas, hide original image
    const canvas = editorContent.querySelector('#shader-canvas');
    if (canvas) {
      canvas.style.display = 'block';
      img.style.display = 'none';
    }
  }
});

// Global shader selection function
async function selectShader(segmentIndex, shaderName) {
  // Update UI
  document.querySelectorAll('.shader-option').forEach(btn => {
    btn.classList.remove('selected');
  });
  document.querySelector(`[data-shader="${shaderName}"]`)?.classList.add('selected');
  
  // Save to backend via fetch (not htmx, to avoid DOM swap)
  const segmentStart = document.getElementById('segment-start');
  if (segmentStart) {
    const idxMatch = window.location.pathname.match(/\/api\/segment\/(\d+)/);
    const segIdx = idxMatch ? idxMatch[1] : 'current';
    try {
      await fetch(`/api/segment/${segIdx}/shader?shader=${shaderName}`, {
        method: 'POST'
      });
    } catch (e) {
      console.error('Failed to save shader:', e);
    }
  }
  
  // Load shader in preview
  const preview = shaderPreviews[0];
  if (!preview) return;
  
  if (shaderName === 'none') {
    preview.cleanup();
    preview.stop();
    // Show original image
    const canvas = document.getElementById('shader-canvas');
    const img = document.getElementById('shader-source-image');
    if (canvas && img) {
      canvas.style.display = 'none';
      img.style.display = 'block';
    }
    return;
  }
  
  // Show canvas, hide original image
  const canvas = document.getElementById('shader-canvas');
  const img = document.getElementById('shader-source-image');
  if (canvas && img) {
    canvas.style.display = 'block';
    img.style.display = 'none';
  }
  
  await preview.loadShader(shaderName);
  preview.start();
}
