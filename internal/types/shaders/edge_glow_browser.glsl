precision mediump float;

uniform float u_time;
uniform vec2 u_resolution;
uniform sampler2D u_texture;

varying vec2 v_texCoord;

void main() {
    vec2 uv = v_texCoord;
    float frame = u_time * 24.0;
    float offset = 0.002;
    
    vec3 center = texture2D(u_texture, uv).rgb;
    vec3 right = texture2D(u_texture, uv + vec2(offset, 0.0)).rgb;
    vec3 down = texture2D(u_texture, uv + vec2(0.0, offset)).rgb;
    
    float edge = length(center - right) + length(center - down);
    edge = smoothstep(0.05, 0.2, edge);
    
    vec3 neon = mix(vec3(0.0, 1.0, 1.0), vec3(1.0, 0.0, 1.0), sin(frame * 0.02) * 0.5 + 0.5);
    
    vec3 finalCol = mix(center * 0.2, neon, edge);
    
    gl_FragColor = vec4(finalCol, 1.0);
}
