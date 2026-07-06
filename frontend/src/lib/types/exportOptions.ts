export interface ExportConfig {
  resolution: '720p' | '1080p' | '4k';
  frameRate: number;
  cameraMode: 'current' | 'isometric' | 'front' | 'top';
  bgColor: string;
  transparent: boolean;
  holdFirst: number;
  holdLast: number;
  fileNaming: string;
  formats: ('mp4' | 'gif' | 'png' | 'zip')[];
  featureLabel: boolean;
  skipSketches: boolean;
  skipSuppressed: boolean;
  skipConstruction: boolean;
  geometryOnly: boolean;
}

export const DEFAULT_EXPORT_CONFIG: ExportConfig = {
  resolution: '1080p',
  frameRate: 24,
  cameraMode: 'isometric',
  bgColor: '#ffffff',
  transparent: false,
  holdFirst: 0,
  holdLast: 0,
  fileNaming: 'frame_{index}',
  formats: ['mp4'],
  featureLabel: false,
  skipSketches: true,
  skipSuppressed: true,
  skipConstruction: false,
  geometryOnly: false
};
