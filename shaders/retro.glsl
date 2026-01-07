//!HOOK MAIN
//!BIND HOOKED
//!DESC Retro VHS Effect with Movement

// Simple noise function for grain/jitter
float noise(vec2 p) {
    return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

vec4 hook() {
    vec2 uv = HOOKED_pos;
    float t = float(frame) * 0.01; // Scaled time

    // 1. Tape Warp (Subtle horizontal waving)
    uv.x += sin(uv.y * 10.0 + t * 2.0) * 0.001;
    uv.x += sin(uv.y * 100.0 + t * 5.0) * 0.0005;

    // 2. Vertical Tracking Displacement
    // Occasionally "tears" a line of the image horizontally
    float lineNoise = step(0.997, noise(vec2(t, uv.y)));
    uv.x += lineNoise * 0.02 * sin(t * 50.0);

    // 3. Chromatic Aberration (Dynamic shift)
    // The shift increases slightly near the edges of the screen
    float edgeDist = length(uv - 0.5);
    float abAmount = 0.003 + (edgeDist * 0.005);
    float r = HOOKED_tex(vec2(uv.x + abAmount, uv.y)).r;
    float g = HOOKED_tex(uv).g;
    float b = HOOKED_tex(vec2(uv.x - abAmount, uv.y)).b;
    
    vec3 col = vec3(r, g, b);

    // 4. Moving Scanlines
    // Added a slight scroll to the scanlines so they don't look static
    float scanline = sin(uv.y * 800.0 + t) * 0.03;
    col -= scanline;

    // 5. Tape Grain (Replaces the strobing)
    // This creates moving "snow" instead of a flashing screen
    float grain = noise(uv + t) * 0.06;
    col += grain;

    // 6. Subtle Vignette (Darkens corners for retro feel)
    col *= 1.0 - (edgeDist * 0.5);

    // 7. Slight Color Grade (Lift blacks, wash out slightly)
    col = mix(col, vec3(0.1, 0.1, 0.15), 0.05); // Blue-ish tint in shadows

    return vec4(col, 1.0);
}