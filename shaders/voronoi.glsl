//!HOOK MAIN
//!BIND HOOKED
//!DESC Geometric Mosaic with Soft Slow Shine

vec4 hook() {
    vec2 uv = HOOKED_pos;
    float scale = 30.0;
    
    // 1. YOUR ORIGINAL GRID LOGIC
    vec2 grid = floor(uv * scale) / scale;
    vec3 col = HOOKED_tex(grid).rgb;
    
    vec2 gv = fract(uv * scale);
    float border = smoothstep(0.0, 0.1, gv.x) * smoothstep(1.0, 0.9, gv.x) *
                   smoothstep(0.0, 0.1, gv.y) * smoothstep(1.0, 0.9, gv.y);
    
    // 2. SLOWER MOVEMENT
    // Reduced from 0.02 to 0.008 for a much calmer pace
    float t = float(frame) * 0.008;
    
    vec2 lightPos = vec2(
        0.5 + 0.3 * sin(t * 0.5), 
        0.5 + 0.3 * cos(t * 0.7)
    );
    
    // 3. SOFTER SHINE
    float dist = distance(uv, lightPos);
    
    // Increasing the first value (0.8) makes the light spread out further
    // and fade much more gradually (softer edges).
    float shine = smoothstep(0.8, 0.0, dist) * 0.4;
    
    // 4. COMBINE
    // We add the shine to the color, then apply the border 
    // This keeps your black grid lines sharp while the tiles "glow"
    vec3 finalCol = (col + shine) * border;
                   
    return vec4(finalCol, 1.0);
}