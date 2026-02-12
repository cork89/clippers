// ./internal/views/static/app.js

// Filter images based on search input
function filterImages(query) {
  const cards = document.querySelectorAll('.image-card');
  const searchLower = query.toLowerCase();
  
  cards.forEach(card => {
    const caption = card.querySelector('.caption')?.textContent?.toLowerCase() || '';
    const tags = Array.from(card.querySelectorAll('.tag')).map(t => t.textContent.toLowerCase());
    
    const matches = caption.includes(searchLower) || tags.some(t => t.includes(searchLower));
    card.style.display = matches ? 'block' : 'none';
  });
}

// Handle save button
document.addEventListener('htmx:afterRequest', (event) => {
  // Handle save response
  if (event.detail.target.id === 'save-btn' || event.detail.requestConfig?.path?.includes('/api/save')) {
    if (event.detail.successful) {
      showNotification('Timeline saved successfully!', 'success');
    } else {
      showNotification('Failed to save timeline', 'error');
    }
  }
  
  // Handle render start
  if (event.detail.target.id === 'render-btn' || event.detail.requestConfig?.path === '/api/render') {
    if (event.detail.successful) {
      document.getElementById('progress-modal').classList.remove('hidden');
    }
  }
});

// WebSocket message handling for progress updates
document.addEventListener('htmx:wsOpen', (event) => {
  console.log('WebSocket connected');
});

document.addEventListener('htmx:wsMessage', (event) => {
  try {
    const data = JSON.parse(event.detail.message.data);
    handleProgressUpdate(data);
  } catch (e) {
    console.error('Failed to parse WebSocket message:', e);
  }
});

// Handle progress updates
function handleProgressUpdate(data) {
  const modal = document.getElementById('progress-modal');
  const progressFill = document.getElementById('progress-fill');
  const progressText = document.getElementById('progress-text');
  
  if (!modal) return;
  
  if (data.stage === 'complete') {
    modal.classList.add('hidden');
    showNotification('Video rendering complete!', 'success');
    return;
  }
  
  if (data.stage === 'error') {
    modal.classList.add('hidden');
    showNotification('Error: ' + data.message, 'error');
    return;
  }
  
  modal.classList.remove('hidden');
  if (progressFill) progressFill.style.width = `${(data.percent || 0) * 100}%`;
  if (progressText) progressText.textContent = data.message || 'Processing...';
}

// Simple notification system
function showNotification(message, type = 'info') {
  // Remove existing notifications
  const existing = document.querySelector('.notification');
  if (existing) existing.remove();
  
  const notification = document.createElement('div');
  notification.className = `notification notification-${type}`;
  notification.textContent = message;
  
  // Add styles
  notification.style.cssText = `
    position: fixed;
    top: 20px;
    right: 20px;
    padding: 1rem 1.5rem;
    border-radius: 6px;
    color: white;
    font-weight: 500;
    z-index: 10000;
    animation: slideIn 0.3s ease;
    background-color: ${type === 'success' ? '#4ecca3' : type === 'error' ? '#ff6b6b' : '#0f3460'};
  `;
  
  document.body.appendChild(notification);
  
  // Remove after 3 seconds
  setTimeout(() => {
    notification.style.animation = 'slideOut 0.3s ease';
    setTimeout(() => notification.remove(), 300);
  }, 3000);
}

// Add animation styles
const style = document.createElement('style');
style.textContent = `
  @keyframes slideIn {
    from { transform: translateX(100%); opacity: 0; }
    to { transform: translateX(0); opacity: 1; }
  }
  @keyframes slideOut {
    from { transform: translateX(0); opacity: 1; }
    to { transform: translateX(100%); opacity: 0; }
  }
`;
document.head.appendChild(style);

// Keyboard shortcuts
document.addEventListener('keydown', (e) => {
  // Ctrl+S to save
  if (e.ctrlKey && e.key === 's') {
    e.preventDefault();
    document.getElementById('save-btn')?.click();
  }
  
  // Escape to close modals
  if (e.key === 'Escape') {
    document.querySelectorAll('.modal').forEach(modal => {
      modal.classList.add('hidden');
    });
  }
});

console.log('Clippers Timeline Editor loaded');
