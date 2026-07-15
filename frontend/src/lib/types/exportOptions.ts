export interface ExportConfig {
  resolution: '720p' | '1080p' | '4k';
  frameRate: number;
  cameraMode: string;
  viewMatrix?: string;
  cameraViewport?: string;
  bgColor: string;
  transparent: boolean;
  holdFirst: number;
  holdLast: number;
  fileNaming: string;
  formats: ('mp4' | 'gif' | 'zip')[];
  featureLabel: boolean;
  skipSketches: boolean;
  skipSuppressed: boolean;
  skipConstruction: boolean;
  geometryOnly: boolean;
  bboxMode: 'once' | 'each';
  zoom: number;
}

export const DEFAULT_EXPORT_CONFIG: ExportConfig = {
  resolution: '1080p',
  frameRate: 6,
  cameraMode: 'isometric',
  viewMatrix: '',
  cameraViewport: '',
  bgColor: '#ffffff',
  transparent: false,
  holdFirst: 0,
  holdLast: 0,
  fileNaming: 'frame_{index}',
  formats: ['mp4', 'zip'],
  featureLabel: false,
  skipSketches: true,
  skipSuppressed: true,
  skipConstruction: false,
  geometryOnly: false,
  bboxMode: 'once',
  zoom: 1
};
