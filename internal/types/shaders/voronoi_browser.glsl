precision mediump float;

uniform float u_time;
uniform vec2 u_resolution;
uniform sampler2D u_texture;

varying vec2 v_texCoord;

void main() {
    vec2 uv = v_texCoord;
    float frame = u_time * 24.0;
    float scale = 30.0;
    
    vec2 grid = floor(uv * scale) / scale;
    vec3 col = texture2D(u_texture, grid).rgb;
    
    vec2 gv = fract(uv * scale);
    float border = smoothstep(0.0, 0.1, gv.x) * smoothstep(1.0, 0.9, gv.x) *
                   smoothstep(0.0, 0.1, gv.y) * smoothstep(1.0, 0.9, gv.y);
    
    float t = frame * 0.008;
    
    vec2 lightPos = vec2(
        0.5 + 0.3 * sin(t * 0.5), 
        0.5 + 0.3 * cos(t * 0.7)
    );
    
    float dist = distance(uv, lightPos);
    
    float shine = smoothstep(0.8, 0.0, dist) * 0.4;
    
    vec3 finalCol = (col + shine) * border;
                   
    gl_FragColor = vec4(finalCol, 1.0);
}
