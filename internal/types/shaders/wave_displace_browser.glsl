precision mediump float;

uniform float u_time;
uniform vec2 u_resolution;
uniform sampler2D u_texture;

varying vec2 v_texCoord;

vec4 hook() {
    vec2 uv = v_texCoord;
    float frame = u_time * 24.0;
    
    vec2 size = u_resolution;
    vec2 pix = uv * size;
    
    float X = pix.x;
    float Y = pix.y;
    float t = frame / 24.0;

    float phaseR = 0.0;
    float phaseG = 2.0;
    float phaseB = 4.0;

    float waveXR = X/40.0 + Y/30.0 + t + phaseR;
    float waveXG = X/40.0 + Y/30.0 + t + phaseG;
    float waveXB = X/40.0 + Y/30.0 + t + phaseB;
    
    float waveYR = X/50.0 + Y/40.0 + t + phaseR;
    float waveYG = X/50.0 + Y/40.0 + t + phaseG;
    float waveYB = X/50.0 + Y/40.0 + t + phaseB;

    vec2 dispR = vec2(12.0 * sin(waveXR), 12.0 * cos(waveYR));
    vec2 dispG = vec2(12.0 * sin(waveXG), 12.0 * cos(waveYG));
    vec2 dispB = vec2(12.0 * sin(waveXB), 12.0 * cos(waveYB));

    vec2 basePix = floor(pix);

    vec2 rPix = basePix + dispR;
    vec2 gPix = basePix + dispG;
    vec2 bPix = basePix + dispB;

    rPix = clamp(rPix, vec2(0.0), size - 1.0);
    gPix = clamp(gPix, vec2(0.0), size - 1.0);
    bPix = clamp(bPix, vec2(0.0), size - 1.0);

    vec2 rUV = rPix / size;
    vec2 gUV = gPix / size;
    vec2 bUV = bPix / size;

    float r = texture2D(u_texture, rUV).r;
    float g = texture2D(u_texture, gUV).g;
    float b = texture2D(u_texture, bUV).b;

    r *= 1.0 + 0.15 * sin(t + phaseR);
    g *= 1.0 + 0.15 * sin(t + phaseG);
    b *= 1.0 + 0.15 * sin(t + phaseB);

    vec3 rgb = floor(vec3(r, g, b) * 255.0) / 255.0;

    return vec4(rgb, 1.0);
}

void main() {
    gl_FragColor = hook();
}
