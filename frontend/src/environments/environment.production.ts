/**
 * Settings for production builds (replaces environment.ts via angular.json
 * fileReplacements). Point apiUrl at the deployed API, either by editing this
 * file or, in CI/Docker, by passing the API_URL build argument (see
 * frontend/Dockerfile and docs/CI-CD.md).
 */
export const environment = {
  apiUrl: 'http://localhost:8080/api',
};
