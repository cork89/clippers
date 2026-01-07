//!HOOK MAIN
//!BIND HOOKED
//!DESC Digital Pixel Melt

vec4 hook() {
    vec2 uv = HOOKED_pos;
    float t = float(frame) * 0.01;

    // Only "melt" specific columns based on a sine wave
    float strength = smoothstep(0.5, 0.6, sin(uv.x * 20.0 + t));
    
    // Shift UVs vertically based on brightness and time
    float brightness = length(HOOKED_tex(uv).rgb);
    uv.y -= strength * brightness * 0.1 * sin(t);

    return HOOKED_tex(uv);
}