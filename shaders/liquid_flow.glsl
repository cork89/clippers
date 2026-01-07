//!HOOK MAIN
//!BIND HOOKED
//!DESC Liquid Pastel Flow (Warping Effect)

vec4 hook() {
    vec2 uv = HOOKED_pos;
    
    // Very slow speed
    float t = float(frame) * 0.008; 

    // Create the "Liquid Marble" warping vector
    vec2 warp = uv;
    for(float i = 1.0; i < 4.0; i++) {
        warp.x += 0.3 / i * sin(i * 3.0 * uv.y + t);
        warp.y += 0.3 / i * cos(i * 3.0 * uv.x + t);
    }

    // 1. Distort the actual video sampling
    // We mix the original UV with the warped UV so the image is still recognizable
    vec2 distortedUV = mix(uv, warp, 0.12); 
    vec4 col = HOOKED_tex(distortedUV);

    // 2. Generate the Pastel Tint (matching your screenshot)
    // This uses the warp math to decide where pink/blue goes
    float pattern = sin(warp.x + warp.y) * 0.5 + 0.5;
    vec3 pink = vec3(1.0, 0.75, 0.9);
    vec3 cyan = vec3(0.7, 0.95, 1.0);
    vec3 pastelTint = mix(pink, cyan, pattern);

    // 3. Blend the colors
    // We use a "Soft Light" style blend to keep the underlying image details
    col.rgb = col.rgb * (pastelTint + 0.5);
    
    // Add the white pearlescent highlights from the screenshot
    float glow = smoothstep(0.8, 1.0, pattern);
    col.rgb += glow * 0.15;

    return col;
}