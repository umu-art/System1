FROM nginx/nginx-unprivileged:1.30.7-alpine

COPY /iac/images/s1-ui/default.conf /etc/nginx/conf.d/default.conf
COPY /modules/ui/dist/ /usr/share/nginx/html/

EXPOSE 8080
