// ./internal/views/static/app.js

// Filter images based on search input
let currentAudioTimeout = undefined;
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

// Audio playback functions
if (typeof segmentEndTime === 'undefined') {
  var segmentEndTime = null;
}

function playSegment() {
  clearTimeout(currentAudioTimeout)
  const audio = document.getElementById('global-audio');
  const startInput = document.getElementById('segment-start');
  const endInput = document.getElementById('segment-end');
  
  if (!audio || !startInput || !endInput) return;
  
  const startTime = parseFloat(startInput.value);
  const endTime = parseFloat(endInput.value);
  
  if (isNaN(startTime) || isNaN(endTime)) return;
  
  audio.currentTime = startTime;
  audio.play();
  currentAudioTimeout = setTimeout(() => {
    audio.pause()
  }, (endTime-startTime)*1000)
}

// Show audio player when page loads
document.addEventListener('DOMContentLoaded', () => {
  const audio = document.getElementById('global-audio');
  if (audio) {
    // Check if audio source loads successfully
    audio.addEventListener('loadeddata', () => {
      audio.style.display = 'inline-block';
    });
    audio.addEventListener('error', () => {
      console.log('Audio not available for this project');
    });
  }
});

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

  // Handle image selection - update timeline cell
  if (event.detail.requestConfig?.path?.includes('/api/segment/current/image')) {
    if (event.detail.successful) {
      const timelineCell = document.querySelector('.timeline-cell.selected');
      if (timelineCell) {
        timelineCell.classList.add('modified');
        const index = timelineCell.dataset.index;
        const editorContent = event.detail.target.closest('#editor-panel-content');
        if (editorContent) {
          const newImg = editorContent.querySelector('.segment-preview img');
          if (newImg) {
            const timelineImg = timelineCell.querySelector('.thumbnail img');
            if (timelineImg) {
              timelineImg.src = newImg.src;
            }
            const imageId = timelineCell.querySelector('.image-id');
            if (imageId) {
              const imgIdMatch = newImg.src.match(/\/api\/image\/([^?]+)/);
              if (imgIdMatch) {
                imageId.textContent = imgIdMatch[1];
              }
            }
          }
        }
      }
    }
  }

  // Handle timeline cell selection - update visual state
  if (event.detail.requestConfig?.path?.match(/\/api\/segment\/\d+$/)) {
    if (event.detail.successful) {
      document.querySelectorAll('.timeline-cell').forEach(cell => {
        cell.classList.remove('selected');
      });
      const index = event.detail.requestConfig.path.match(/\/api\/segment\/(\d+)/);
      if (index) {
        const cell = document.querySelector(`.timeline-cell[data-index="${index[1]}"]`);
        if (cell) {
          cell.classList.add('selected');
        }
      }
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

// Add animation styles (guard against multiple inclusions)
const style = document.createElement('style');
if (!document.getElementById('clippers-animations')) {
  style.id = 'clippers-animations';
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
}

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
