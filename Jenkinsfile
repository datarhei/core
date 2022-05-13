pipeline {
  agent any
  triggers {
    cron('H 0 * * 6')
  }
  options {
    buildDiscarder(logRotator(numToKeepStr: '5', artifactNumToKeepStr: '5'))
  }
  stages {
    stage('Build AMD64') {
      steps {
        sh 'docker build -f Dockerfile -t docker-registry.marathon.l4lb.thisdcos.directory:5000/core:amd64 .'
        sh 'docker push docker-registry.marathon.l4lb.thisdcos.directory:5000/core:amd64'
        sh 'docker rmi docker-registry.marathon.l4lb.thisdcos.directory:5000/core:amd64'
      }
    }
  }
}
