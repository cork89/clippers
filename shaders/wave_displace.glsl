//!HOOK MAIN
//!BIND HOOKED
//!DESC CPU geq wave emulation (matching original)

vec4 hook() {
    vec2 size = vec2(HOOKED_size);
    vec2 uv = HOOKED_pos;
    vec2 pix = uv * size; // Convert to pixel coordinates
    
    float X = pix.x;
    float Y = pix.y;
    float t = float(frame) / 24.0; // Match N/30 from geq

    // Phase offsets for each channel (R=0, G=2, B=4)
    float phaseR = 0.0;
    float phaseG = 2.0;
    float phaseB = 4.0;

    // Calculate wave displacement for each channel (matching geq parameters exactly)
    float waveXR = X/40.0 + Y/30.0 + t + phaseR;
    float waveXG = X/40.0 + Y/30.0 + t + phaseG;
    float waveXB = X/40.0 + Y/30.0 + t + phaseB;
    
    float waveYR = X/50.0 + Y/40.0 + t + phaseR;
    float waveYG = X/50.0 + Y/40.0 + t + phaseG;
    float waveYB = X/50.0 + Y/40.0 + t + phaseB;

    // Displacement vectors for each channel
    vec2 dispR = vec2(12.0 * sin(waveXR), 12.0 * cos(waveYR));
    vec2 dispG = vec2(12.0 * sin(waveXG), 12.0 * cos(waveYG));
    vec2 dispB = vec2(12.0 * sin(waveXB), 12.0 * cos(waveYB));

    // Base pixel position (floor for integer snapping like geq)
    vec2 basePix = floor(pix);

    // Calculate displaced pixel positions for each channel
    vec2 rPix = basePix + dispR;
    vec2 gPix = basePix + dispG;
    vec2 bPix = basePix + dispB;

    // Clamp to valid pixel coordinates
    rPix = clamp(rPix, vec2(0.0), size - 1.0);
    gPix = clamp(gPix, vec2(0.0), size - 1.0);
    bPix = clamp(bPix, vec2(0.0), size - 1.0);

    // Convert back to UV coordinates for sampling
    vec2 rUV = rPix / size;
    vec2 gUV = gPix / size;
    vec2 bUV = bPix / size;

    // Sample each channel from displaced position
    float r = HOOKED_tex(rUV).r;
    float g = HOOKED_tex(gUV).g;
    float b = HOOKED_tex(bUV).b;

    // Color modulation (matching geq's 1+0.15*sin(N/30+offset))
    r *= 1.0 + 0.15 * sin(t + phaseR);
    g *= 1.0 + 0.15 * sin(t + phaseG);
    b *= 1.0 + 0.15 * sin(t + phaseB);

    // 8-bit quantization (optional, for authentic geq look)
    vec3 rgb = floor(vec3(r, g, b) * 255.0) / 255.0;

    return vec4(rgb, 1.0);
}