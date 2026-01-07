//!HOOK MAIN
//!BIND HOOKED
//!DESC Neon Edge Glow

vec4 hook() {
    vec2 uv = HOOKED_pos;
    float offset = 0.002;
    
    // Sample neighbors to find edges
    vec3 center = HOOKED_tex(uv).rgb;
    vec3 right = HOOKED_tex(uv + vec2(offset, 0.0)).rgb;
    vec3 down = HOOKED_tex(uv + vec2(0.0, offset)).rgb;
    
    // Edge detection (Sobel-lite)
    float edge = length(center - right) + length(center - down);
    edge = smoothstep(0.05, 0.2, edge);
    
    // Neon Color (Cyan/Pink pulse)
    vec3 neon = mix(vec3(0.0, 1.0, 1.0), vec3(1.0, 0.0, 1.0), sin(float(frame) * 0.02) * 0.5 + 0.5);
    
    // Combine: Darken background, brighten edges
    vec3 finalCol = mix(center * 0.2, neon, edge);
    
    return vec4(finalCol, 1.0);
}