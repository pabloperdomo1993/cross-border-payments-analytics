# One public ALB per environment, routing by path prefix to whichever
# of the two ALB-fronted services (payments-service, analytics-service)
# owns that path. payment-processor is never registered here — it has
# no public HTTP API (see modules/ecs-service's attach_to_alb flag).

resource "aws_lb" "this" {
  name               = "${var.name_prefix}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [var.security_group_id]
  subnets            = var.public_subnet_ids

  # Prod gets deletion protection so a `terraform destroy` (or an
  # accidental console click) can't take down the public entry point
  # without a deliberate, separate step; dev/staging don't need that
  # friction.
  enable_deletion_protection = var.enable_deletion_protection

  tags = merge(var.tags, { Name = "${var.name_prefix}-alb" })
}

resource "aws_lb_target_group" "payments_service" {
  name        = "${var.name_prefix}-payments-svc-tg"
  port        = var.payments_service_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip" # required for Fargate awsvpc networking

  health_check {
    path                = "/health"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 15
    timeout             = 5
    matcher             = "200"
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-payments-service-tg" })
}

resource "aws_lb_target_group" "analytics_service" {
  name        = "${var.name_prefix}-analytics-svc-tg"
  port        = var.analytics_service_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    path                = "/health"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 15
    timeout             = 5
    matcher             = "200"
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-analytics-service-tg" })
}

# Plain HTTP is redirected straight to HTTPS — nothing is ever served
# unencrypted, even accidentally.
resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.acm_certificate_arn

  # No sensible default service to fall back to — an unmatched path is a
  # client error, not silently routed to one service or the other.
  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "Not found"
      status_code  = "404"
    }
  }
}

resource "aws_lb_listener_rule" "payments_service" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 100

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.payments_service.arn
  }

  # /health and /ready are deliberately NOT routed here: the target
  # group's own health_check block above already probes each service's
  # /health directly (bypassing listener rules entirely), and both
  # services expose identical /health,/ready paths — routing them
  # externally through one shared, ambiguous rule would silently hide
  # whichever service didn't happen to win listener-rule priority.
  condition {
    path_pattern {
      values = ["/api/v1/transactions*"]
    }
  }
}

resource "aws_lb_listener_rule" "analytics_service" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 200

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.analytics_service.arn
  }

  condition {
    path_pattern {
      values = ["/api/v1/analytics/*"]
    }
  }
}
