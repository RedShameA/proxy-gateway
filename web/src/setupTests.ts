import '@testing-library/jest-dom';

const mockCanvasContext = {
  fillRect: () => {},
  clearRect: () => {},
  getImageData: () => ({ data: new Uint8ClampedArray(4) }),
  putImageData: () => {},
  createImageData: () => [],
  setTransform: () => {},
  drawImage: () => {},
  save: () => {},
  fillText: () => {},
  restore: () => {},
  beginPath: () => {},
  moveTo: () => {},
  lineTo: () => {},
  closePath: () => {},
  stroke: () => {},
  translate: () => {},
  scale: () => {},
  rotate: () => {},
  arc: () => {},
  fill: () => {},
  measureText: () => ({ width: 0 }),
  transform: () => {},
  rect: () => {},
  clip: () => {},
} as unknown as CanvasRenderingContext2D;

Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', {
  value: ((() => mockCanvasContext) as unknown) as typeof HTMLCanvasElement.prototype.getContext,
});
